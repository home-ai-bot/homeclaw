package api

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

// go2rtcProcess holds the state for the managed go2rtc process.
var go2rtcProcess = struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	owned         bool // true if we started the process, false if we attached to an existing one
	runtimeStatus string
	logs          *LogBuffer
}{
	runtimeStatus: "stopped",
	logs:          NewLogBuffer(200),
}

var (
	go2rtcRestartGracePeriod     = 5 * time.Second
	go2rtcRestartForceKillWindow = 3 * time.Second
	go2rtcRestartPollInterval    = 100 * time.Millisecond
)

// registerGo2RTCRoutes binds go2rtc lifecycle endpoints to the ServeMux.
func (h *Handler) registerGo2RTCRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/go2rtc/status", h.handleGo2RTCStatus)
	mux.HandleFunc("GET /api/go2rtc/logs", h.handleGo2RTCLogs)
	mux.HandleFunc("POST /api/go2rtc/logs/clear", h.handleGo2RTCClearLogs)
	mux.HandleFunc("POST /api/go2rtc/start", h.handleGo2RTCStart)
	mux.HandleFunc("POST /api/go2rtc/stop", h.handleGo2RTCStop)
	mux.HandleFunc("POST /api/go2rtc/restart", h.handleGo2RTCRestart)
}

// TryAutoStartGo2RTC checks whether go2rtc start preconditions are met and
// starts it when possible. Intended to be called by the backend at startup.
func (h *Handler) TryAutoStartGo2RTC() {
	go2rtcProcess.mu.Lock()
	defer go2rtcProcess.mu.Unlock()

	if go2rtcProcess.cmd != nil && go2rtcProcess.cmd.Process != nil {
		go2rtcProcess.cmd = nil
	}

	ready, reason, err := h.go2rtcStartReady()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Skip auto-starting go2rtc: %v", err))
		return
	}
	if !ready {
		logger.InfoC("go2rtc", fmt.Sprintf("Skip auto-starting go2rtc: %s", reason))
		return
	}

	pid, err := h.startGo2RTCLocked()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Failed to auto-start go2rtc: %v", err))
		return
	}
	logger.InfoC("go2rtc", fmt.Sprintf("go2rtc auto-started (PID: %d)", pid))
}

// go2rtcStartReady validates whether current config can start go2rtc.
func (h *Handler) go2rtcStartReady() (bool, string, error) {
	configPath := hcconfig.GetGo2RTCPath()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return false, fmt.Sprintf("go2rtc config file not found: %s", configPath), nil
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

func isGo2RTCProcessAliveLocked(cmd *exec.Cmd) bool {
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

func setGo2RTCRuntimeStatusLocked(status string) {
	go2rtcProcess.runtimeStatus = status
}

func waitForGo2RTCProcessExit(cmd *exec.Cmd, timeout time.Duration) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}

	deadline := time.Now().Add(timeout)
	for {
		if !isGo2RTCProcessAliveLocked(cmd) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(go2rtcRestartPollInterval)
	}
}

// StopGo2RTC stops the go2rtc process if it was started by this handler.
// This method is called during application shutdown to ensure the go2rtc subprocess
// is properly terminated. It only stops processes that were started by this handler,
// not processes that were attached to from existing instances.
func (h *Handler) StopGo2RTC() {
	go2rtcProcess.mu.Lock()
	defer go2rtcProcess.mu.Unlock()

	// Only stop if we own the process (started it ourselves)
	if !go2rtcProcess.owned || go2rtcProcess.cmd == nil || go2rtcProcess.cmd.Process == nil {
		return
	}

	pid, err := stopGo2RTCLocked()
	if err != nil {
		logger.ErrorC("go2rtc", fmt.Sprintf("Failed to stop go2rtc (PID %d): %v", pid, err))
		return
	}

	logger.InfoC("go2rtc", fmt.Sprintf("go2rtc stopped (PID: %d)", pid))
}

// stopGo2RTCLocked sends a stop signal to the go2rtc process.
// Assumes go2rtcProcess.mu is held by the caller.
// Returns the PID of the stopped process and any error encountered.
func stopGo2RTCLocked() (int, error) {
	if go2rtcProcess.cmd == nil || go2rtcProcess.cmd.Process == nil {
		return 0, nil
	}

	pid := go2rtcProcess.cmd.Process.Pid

	// Send SIGTERM for graceful shutdown (SIGKILL on Windows)
	var sigErr error
	if runtime.GOOS == "windows" {
		sigErr = go2rtcProcess.cmd.Process.Kill()
	} else {
		sigErr = go2rtcProcess.cmd.Process.Signal(syscall.SIGTERM)
	}

	if sigErr != nil {
		return pid, sigErr
	}

	logger.InfoC("go2rtc", fmt.Sprintf("Sent stop signal to go2rtc (PID: %d)", pid))
	go2rtcProcess.cmd = nil
	go2rtcProcess.owned = false
	setGo2RTCRuntimeStatusLocked("stopped")

	return pid, nil
}

func stopGo2RTCProcessForRestart(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || !isGo2RTCProcessAliveLocked(cmd) {
		return nil
	}

	var stopErr error
	if runtime.GOOS == "windows" {
		stopErr = cmd.Process.Kill()
	} else {
		stopErr = cmd.Process.Signal(syscall.SIGTERM)
	}
	if stopErr != nil && isGo2RTCProcessAliveLocked(cmd) {
		return fmt.Errorf("failed to stop existing go2rtc: %w", stopErr)
	}

	if waitForGo2RTCProcessExit(cmd, go2rtcRestartGracePeriod) {
		return nil
	}

	if runtime.GOOS != "windows" {
		killErr := cmd.Process.Signal(syscall.SIGKILL)
		if killErr != nil && isGo2RTCProcessAliveLocked(cmd) {
			return fmt.Errorf("failed to force-stop existing go2rtc: %w", killErr)
		}
		if waitForGo2RTCProcessExit(cmd, go2rtcRestartForceKillWindow) {
			return nil
		}
	}

	return fmt.Errorf("existing go2rtc did not exit before restart")
}

func (h *Handler) startGo2RTCLocked() (int, error) {
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
	go2rtcProcess.logs.Reset()

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start go2rtc: %w", err)
	}

	go2rtcProcess.cmd = cmd
	go2rtcProcess.owned = true // We started this process
	setGo2RTCRuntimeStatusLocked("running")
	pid := cmd.Process.Pid
	logger.InfoC("go2rtc", fmt.Sprintf("Started go2rtc (PID: %d) from %s with config %s", pid, execPath, configPath))

	// Capture stdout/stderr in background
	go scanGo2RTCPipe(stdoutPipe, go2rtcProcess.logs)
	go scanGo2RTCPipe(stderrPipe, go2rtcProcess.logs)

	// Wait for exit in background and clean up
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.ErrorC("go2rtc", fmt.Sprintf("go2rtc process exited: %v", err))
		} else {
			logger.InfoC("go2rtc", "go2rtc process exited normally")
		}

		go2rtcProcess.mu.Lock()
		if go2rtcProcess.cmd == cmd {
			go2rtcProcess.cmd = nil
			setGo2RTCRuntimeStatusLocked("stopped")
		}
		go2rtcProcess.mu.Unlock()
	}()

	return pid, nil
}

// handleGo2RTCStart starts the go2rtc subprocess.
//
//	POST /api/go2rtc/start
func (h *Handler) handleGo2RTCStart(w http.ResponseWriter, r *http.Request) {
	go2rtcProcess.mu.Lock()
	defer go2rtcProcess.mu.Unlock()

	if go2rtcProcess.cmd != nil && go2rtcProcess.cmd.Process != nil && isGo2RTCProcessAliveLocked(go2rtcProcess.cmd) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "already_running",
			"pid":    go2rtcProcess.cmd.Process.Pid,
		})
		return
	}

	// Clear any stale cmd reference
	if go2rtcProcess.cmd != nil {
		go2rtcProcess.cmd = nil
		setGo2RTCRuntimeStatusLocked("stopped")
	}

	ready, reason, err := h.go2rtcStartReady()
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

	pid, err := h.startGo2RTCLocked()
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

// handleGo2RTCStop stops the running go2rtc subprocess gracefully.
//
//	POST /api/go2rtc/stop
func (h *Handler) handleGo2RTCStop(w http.ResponseWriter, r *http.Request) {
	go2rtcProcess.mu.Lock()
	defer go2rtcProcess.mu.Unlock()

	if go2rtcProcess.cmd == nil || go2rtcProcess.cmd.Process == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "not_running",
		})
		return
	}

	pid, err := stopGo2RTCLocked()
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

// RestartGo2RTC restarts the go2rtc process. This is a non-blocking operation
// that stops the current go2rtc (if running) and starts a new one.
// Returns the PID of the new go2rtc process or an error.
func (h *Handler) RestartGo2RTC() (int, error) {
	ready, reason, err := h.go2rtcStartReady()
	if err != nil {
		return 0, fmt.Errorf("failed to validate go2rtc start conditions: %w", err)
	}
	if !ready {
		return 0, &go2rtcPreconditionFailedError{reason: reason}
	}

	go2rtcProcess.mu.Lock()
	previousCmd := go2rtcProcess.cmd
	setGo2RTCRuntimeStatusLocked("restarting")
	go2rtcProcess.mu.Unlock()

	if err = stopGo2RTCProcessForRestart(previousCmd); err != nil {
		go2rtcProcess.mu.Lock()
		if go2rtcProcess.cmd == previousCmd {
			if isGo2RTCProcessAliveLocked(previousCmd) {
				setGo2RTCRuntimeStatusLocked("running")
			} else {
				go2rtcProcess.cmd = nil
				setGo2RTCRuntimeStatusLocked("error")
			}
		}
		go2rtcProcess.mu.Unlock()
		return 0, fmt.Errorf("failed to stop go2rtc: %w", err)
	}

	go2rtcProcess.mu.Lock()
	if go2rtcProcess.cmd == previousCmd {
		go2rtcProcess.cmd = nil
	}
	pid, err := h.startGo2RTCLocked()
	if err != nil {
		go2rtcProcess.cmd = nil
		setGo2RTCRuntimeStatusLocked("error")
	}
	go2rtcProcess.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("failed to start go2rtc: %w", err)
	}

	return pid, nil
}

// go2rtcPreconditionFailedError is returned when go2rtc restart preconditions are not met
type go2rtcPreconditionFailedError struct {
	reason string
}

func (e *go2rtcPreconditionFailedError) Error() string {
	return e.reason
}

// IsBadRequest returns true if the error should result in a 400 Bad Request status
func (e *go2rtcPreconditionFailedError) IsBadRequest() bool {
	return true
}

// handleGo2RTCRestart stops go2rtc (if running) and starts a new instance.
//
//	POST /api/go2rtc/restart
func (h *Handler) handleGo2RTCRestart(w http.ResponseWriter, r *http.Request) {
	pid, err := h.RestartGo2RTC()
	if err != nil {
		// Check if it's a precondition failed error
		var precondErr *go2rtcPreconditionFailedError
		if ok := isGo2RTCPreconditionError(err, &precondErr); ok {
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

func isGo2RTCPreconditionError(err error, target **go2rtcPreconditionFailedError) bool {
	for err != nil {
		if e, ok := err.(*go2rtcPreconditionFailedError); ok {
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

// handleGo2RTCClearLogs clears the in-memory go2rtc log buffer.
//
//	POST /api/go2rtc/logs/clear
func (h *Handler) handleGo2RTCClearLogs(w http.ResponseWriter, r *http.Request) {
	go2rtcProcess.logs.Clear()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "cleared",
		"log_total":  0,
		"log_run_id": go2rtcProcess.logs.RunID(),
	})
}

// handleGo2RTCStatus returns the go2rtc run status.
//
//	GET /api/go2rtc/status
func (h *Handler) handleGo2RTCStatus(w http.ResponseWriter, r *http.Request) {
	data := h.go2rtcStatusData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) go2rtcStatusData() map[string]any {
	data := map[string]any{}

	go2rtcProcess.mu.Lock()
	defer go2rtcProcess.mu.Unlock()

	// Check if process is still alive
	if go2rtcProcess.cmd != nil && go2rtcProcess.cmd.Process != nil {
		if isGo2RTCProcessAliveLocked(go2rtcProcess.cmd) {
			data["go2rtc_status"] = "running"
			data["pid"] = go2rtcProcess.cmd.Process.Pid
		} else {
			data["go2rtc_status"] = "stopped"
			go2rtcProcess.cmd = nil
			setGo2RTCRuntimeStatusLocked("stopped")
		}
	} else {
		data["go2rtc_status"] = go2rtcProcess.runtimeStatus
	}

	// Add config path info
	data["config_path"] = hcconfig.GetGo2RTCPath()
	data["binary_path"] = findGo2RTCBinary()

	ready, reason, readyErr := h.go2rtcStartReady()
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

// handleGo2RTCLogs returns buffered go2rtc logs, optionally incrementally.
//
//	GET /api/go2rtc/logs
func (h *Handler) handleGo2RTCLogs(w http.ResponseWriter, r *http.Request) {
	data := go2rtcLogsData(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// go2rtcLogsData reads log_offset and log_run_id query params from the request
// and returns incremental log lines.
func go2rtcLogsData(r *http.Request) map[string]any {
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

	runID := go2rtcProcess.logs.RunID()

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

	lines, total, runID := go2rtcProcess.logs.LinesSince(offset)
	if lines == nil {
		lines = []string{}
	}

	data["logs"] = lines
	data["log_total"] = total
	data["log_run_id"] = runID
	return data
}

// scanGo2RTCPipe reads lines from r and appends them to buf. Returns when r reaches EOF.
func scanGo2RTCPipe(r io.Reader, buf *LogBuffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		buf.Append(scanner.Text())
	}
}
