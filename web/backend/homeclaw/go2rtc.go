package homeclaw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	hcconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/web/backend/utils"
)

// Go2RTCManager manages the go2rtc subprocess lifecycle.
type Go2RTCManager struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	owned         bool // true if we started the process, false if we attached to an existing one
	runtimeStatus string
	logs          *LogBuffer
}

var (
	go2rtcRestartGracePeriod     = 5 * time.Second
	go2rtcRestartForceKillWindow = 3 * time.Second
	go2rtcRestartPollInterval    = 100 * time.Millisecond
)

// NewGo2RTCManager creates a new Go2RTCManager instance.
func NewGo2RTCManager() *Go2RTCManager {
	return &Go2RTCManager{
		runtimeStatus: "stopped",
		logs:          NewLogBuffer(200),
	}
}

// RegisterRoutes binds go2rtc lifecycle endpoints to the ServeMux.
func (m *Go2RTCManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/go2rtc/status", m.handleStatus)
	mux.HandleFunc("GET /api/go2rtc/logs", m.handleLogs)
	mux.HandleFunc("POST /api/go2rtc/logs/clear", m.handleClearLogs)
	mux.HandleFunc("POST /api/go2rtc/start", m.handleStart)
	mux.HandleFunc("POST /api/go2rtc/stop", m.handleStop)
	mux.HandleFunc("POST /api/go2rtc/restart", m.handleRestart)
}

// TryAutoStart checks whether go2rtc start preconditions are met and
// starts it when possible. Intended to be called by the backend at startup.
func (m *Go2RTCManager) TryAutoStart() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd = nil
	}

	ready, reason, err := m.startReady()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Skip auto-starting go2rtc: %v", err))
		return
	}
	if !ready {
		logger.InfoC("go2rtc", fmt.Sprintf("Skip auto-starting go2rtc: %s", reason))
		return
	}

	pid, err := m.startLocked()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Failed to auto-start go2rtc: %v", err))
		return
	}
	logger.InfoC("go2rtc", fmt.Sprintf("go2rtc auto-started (PID: %d)", pid))
}

// startReady validates whether current config can start go2rtc.
func (m *Go2RTCManager) startReady() (bool, string, error) {
	configPath := hcconfig.GetGo2RTCPath()

	// Check if config file exists, create default config if not
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file so go2rtc can start
		defaultConfig := "api:\n  listen: \":1984\"\n"
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return false, "", fmt.Errorf("failed to create go2rtc config file: %w", err)
		}
		logger.InfoC("go2rtc", fmt.Sprintf("Created default config file: %s", configPath))
	}

	// Check if go2rtc binary exists
	binaryPath := findGo2RTCBinary()
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return false, fmt.Sprintf("go2rtc binary not found: %s", binaryPath), nil
	}

	return true, "", nil
}

// findGo2RTCBinary locates the go2rtc executable.
// Search order:
//  1. Same directory as the current executable
//  2. Falls back to "go2rtc" and relies on $PATH
func findGo2RTCBinary() string {
	binaryName := "go2rtc"
	if runtime.GOOS == "windows" {
		binaryName = "go2rtc.exe"
	}

	// Check same directory as picoclaw binary
	picoclawBinary := utils.FindPicoclawBinary()
	if picoclawBinary != "picoclaw" && picoclawBinary != "picoclaw.exe" {
		candidate := filepath.Join(filepath.Dir(picoclawBinary), binaryName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	// Check same directory as current executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), binaryName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return binaryName
}

func (m *Go2RTCManager) isProcessAliveLocked(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}

	// Wait() sets ProcessState when the process exits; use it when available.
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return false
	}

	// Windows does not support Signal(0) probing. If we still own cmd and it
	// has not reported exit, treat it as alive.
	if runtime.GOOS == "windows" {
		return true
	}

	return cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (m *Go2RTCManager) setRuntimeStatusLocked(status string) {
	m.runtimeStatus = status
}

func (m *Go2RTCManager) waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}

	deadline := time.Now().Add(timeout)
	for {
		if !m.isProcessAliveLocked(cmd) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(go2rtcRestartPollInterval)
	}
}

// Stop stops the go2rtc process if it was started by this manager.
// This method is called during application shutdown to ensure the go2rtc subprocess
// is properly terminated. It only stops processes that were started by this manager,
// not processes that were attached to from existing instances.
func (m *Go2RTCManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only stop if we own the process (started it ourselves)
	if !m.owned || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	pid, err := m.stopLocked()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Failed to stop go2rtc (PID %d): %v", pid, err))
		return
	}

	logger.InfoC("go2rtc", fmt.Sprintf("go2rtc stopped (PID: %d)", pid))
}

// stopLocked sends a stop signal to the go2rtc process.
// Assumes m.mu is held by the caller.
// Returns the PID of the stopped process and any error encountered.
func (m *Go2RTCManager) stopLocked() (int, error) {
	if m.cmd == nil || m.cmd.Process == nil {
		return 0, nil
	}

	pid := m.cmd.Process.Pid

	// Send SIGTERM for graceful shutdown (SIGKILL on Windows)
	var sigErr error
	if runtime.GOOS == "windows" {
		sigErr = m.cmd.Process.Kill()
	} else {
		sigErr = m.cmd.Process.Signal(syscall.SIGTERM)
	}

	if sigErr != nil {
		return pid, sigErr
	}

	logger.InfoC("go2rtc", fmt.Sprintf("Sent stop signal to go2rtc (PID: %d)", pid))
	m.cmd = nil
	m.owned = false
	m.setRuntimeStatusLocked("stopped")

	return pid, nil
}

func (m *Go2RTCManager) stopProcessForRestart(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || !m.isProcessAliveLocked(cmd) {
		return nil
	}

	var stopErr error
	if runtime.GOOS == "windows" {
		stopErr = cmd.Process.Kill()
	} else {
		stopErr = cmd.Process.Signal(syscall.SIGTERM)
	}
	if stopErr != nil && m.isProcessAliveLocked(cmd) {
		return fmt.Errorf("failed to stop existing go2rtc: %w", stopErr)
	}

	if m.waitForProcessExit(cmd, go2rtcRestartGracePeriod) {
		return nil
	}

	if runtime.GOOS != "windows" {
		killErr := cmd.Process.Signal(syscall.SIGKILL)
		if killErr != nil && m.isProcessAliveLocked(cmd) {
			return fmt.Errorf("failed to force-stop existing go2rtc: %w", killErr)
		}
		if m.waitForProcessExit(cmd, go2rtcRestartForceKillWindow) {
			return nil
		}
	}

	return fmt.Errorf("existing go2rtc did not exit before restart")
}

func (m *Go2RTCManager) startLocked() (int, error) {
	configPath := hcconfig.GetGo2RTCPath()

	// Locate the go2rtc executable
	execPath := findGo2RTCBinary()

	cmd := exec.Command(execPath, "-c", configPath)
	cmd.Env = os.Environ()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Clear old logs for this new run
	m.logs.Reset()

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start go2rtc: %w", err)
	}

	m.cmd = cmd
	m.owned = true // We started this process
	m.setRuntimeStatusLocked("running")
	pid := cmd.Process.Pid
	logger.InfoC("go2rtc", fmt.Sprintf("Started go2rtc (PID: %d) from %s with config %s", pid, execPath, configPath))

	// Capture stdout/stderr in background
	go scanPipe(stdoutPipe, m.logs)
	go scanPipe(stderrPipe, m.logs)

	// Wait for exit in background and clean up
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.ErrorC("go2rtc", fmt.Sprintf("go2rtc process exited: %v", err))
		} else {
			logger.InfoC("go2rtc", "go2rtc process exited normally")
		}

		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
			m.setRuntimeStatusLocked("stopped")
		}
		m.mu.Unlock()
	}()

	return pid, nil
}

// handleStart starts the go2rtc subprocess.
//
//	POST /api/go2rtc/start
func (m *Go2RTCManager) handleStart(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.Process != nil && m.isProcessAliveLocked(m.cmd) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "already_running",
			"pid":    m.cmd.Process.Pid,
		})
		return
	}

	// Clear any stale cmd reference
	if m.cmd != nil {
		m.cmd = nil
		m.setRuntimeStatusLocked("stopped")
	}

	ready, reason, err := m.startReady()
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to validate go2rtc start conditions: %v", err),
			http.StatusInternalServerError,
		)
		return
	}
	if !ready {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "precondition_failed",
			"message": reason,
		})
		return
	}

	pid, err := m.startLocked()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start go2rtc: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"pid":    pid,
	})
}

// handleStop stops the running go2rtc subprocess gracefully.
//
//	POST /api/go2rtc/stop
func (m *Go2RTCManager) handleStop(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "not_running",
		})
		return
	}

	pid, err := m.stopLocked()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop go2rtc (PID %d): %v", pid, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"pid":    pid,
	})
}

// Restart restarts the go2rtc process. This is a non-blocking operation
// that stops the current go2rtc (if running) and starts a new one.
// Returns the PID of the new go2rtc process or an error.
func (m *Go2RTCManager) Restart() (int, error) {
	ready, reason, err := m.startReady()
	if err != nil {
		return 0, fmt.Errorf("failed to validate go2rtc start conditions: %w", err)
	}
	if !ready {
		return 0, &PreconditionFailedError{reason: reason}
	}

	m.mu.Lock()
	previousCmd := m.cmd
	m.setRuntimeStatusLocked("restarting")
	m.mu.Unlock()

	if err = m.stopProcessForRestart(previousCmd); err != nil {
		m.mu.Lock()
		if m.cmd == previousCmd {
			if m.isProcessAliveLocked(previousCmd) {
				m.setRuntimeStatusLocked("running")
			} else {
				m.cmd = nil
				m.setRuntimeStatusLocked("error")
			}
		}
		m.mu.Unlock()
		return 0, fmt.Errorf("failed to stop go2rtc: %w", err)
	}

	m.mu.Lock()
	if m.cmd == previousCmd {
		m.cmd = nil
	}
	pid, err := m.startLocked()
	if err != nil {
		m.cmd = nil
		m.setRuntimeStatusLocked("error")
	}
	m.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("failed to start go2rtc: %w", err)
	}

	return pid, nil
}

// PreconditionFailedError is returned when go2rtc restart preconditions are not met
type PreconditionFailedError struct {
	reason string
}

func (e *PreconditionFailedError) Error() string {
	return e.reason
}

// IsBadRequest returns true if the error should result in a 400 Bad Request status
func (e *PreconditionFailedError) IsBadRequest() bool {
	return true
}

// handleRestart stops go2rtc (if running) and starts a new instance.
//
//	POST /api/go2rtc/restart
func (m *Go2RTCManager) handleRestart(w http.ResponseWriter, r *http.Request) {
	pid, err := m.Restart()
	if err != nil {
		// Check if it's a precondition failed error
		var precondErr *PreconditionFailedError
		if ok := isPreconditionError(err, &precondErr); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"status":  "precondition_failed",
				"message": precondErr.reason,
			})
			return
		}
		http.Error(w, fmt.Sprintf("Failed to restart go2rtc: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"pid":    pid,
	})
}

func isPreconditionError(err error, target **PreconditionFailedError) bool {
	for err != nil {
		if e, ok := err.(*PreconditionFailedError); ok {
			*target = e
			return true
		}
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			err = unwrapper.Unwrap()
		} else {
			break
		}
	}
	return false
}

// handleClearLogs clears the in-memory go2rtc log buffer.
//
//	POST /api/go2rtc/logs/clear
func (m *Go2RTCManager) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	m.logs.Clear()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "cleared",
		"log_total":  0,
		"log_run_id": m.logs.RunID(),
	})
}

// handleStatus returns the go2rtc run status.
//
//	GET /api/go2rtc/status
func (m *Go2RTCManager) handleStatus(w http.ResponseWriter, r *http.Request) {
	data := m.statusData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (m *Go2RTCManager) statusData() map[string]any {
	data := map[string]any{}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if process is still alive
	if m.cmd != nil && m.cmd.Process != nil {
		if m.isProcessAliveLocked(m.cmd) {
			data["go2rtc_status"] = "running"
			data["pid"] = m.cmd.Process.Pid
		} else {
			data["go2rtc_status"] = "stopped"
			m.cmd = nil
			m.setRuntimeStatusLocked("stopped")
		}
	} else {
		data["go2rtc_status"] = m.runtimeStatus
	}

	// Add config path info
	data["config_path"] = hcconfig.GetGo2RTCPath()
	data["binary_path"] = findGo2RTCBinary()

	ready, reason, readyErr := m.startReady()
	if readyErr != nil {
		data["go2rtc_start_allowed"] = false
		data["go2rtc_start_reason"] = readyErr.Error()
	} else {
		data["go2rtc_start_allowed"] = ready
		if !ready {
			data["go2rtc_start_reason"] = reason
		}
	}

	return data
}

// handleLogs returns buffered go2rtc logs, optionally incrementally.
//
//	GET /api/go2rtc/logs
func (m *Go2RTCManager) handleLogs(w http.ResponseWriter, r *http.Request) {
	data := m.logsData(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// logsData reads log_offset and log_run_id query params from the request
// and returns incremental log lines.
func (m *Go2RTCManager) logsData(r *http.Request) map[string]any {
	data := map[string]any{}
	clientOffset := 0
	clientRunID := -1

	if v := r.URL.Query().Get("log_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			clientOffset = n
		}
	}

	if v := r.URL.Query().Get("log_run_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			clientRunID = n
		}
	}

	runID := m.logs.RunID()

	if runID == 0 {
		data["logs"] = []string{}
		data["log_total"] = 0
		data["log_run_id"] = 0
		return data
	}

	// If runID changed, reset offset to get all logs from new run
	offset := clientOffset
	if clientRunID != runID {
		offset = 0
	}

	lines, total, runID := m.logs.LinesSince(offset)
	if lines == nil {
		lines = []string{}
	}

	data["logs"] = lines
	data["log_total"] = total
	data["log_run_id"] = runID
	return data
}

// scanPipe reads lines from r and appends them to buf. Returns when r reaches EOF.
func scanPipe(r io.Reader, buf *LogBuffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		buf.Append(scanner.Text())
	}
}
