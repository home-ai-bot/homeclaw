// Package common provides common utility functions.
package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	hcconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
)

// cachedFFmpegPath stores the resolved ffmpeg binary path.
var (
	cachedFFmpegPath string
	ffmpegPathOnce   sync.Once
)

// findFFmpegBinary locates the ffmpeg executable (cached after first call).
// Search order:
//  1. Same directory as the current executable
//  2. Falls back to "ffmpeg" and relies on $PATH
func findFFmpegBinary() string {
	ffmpegPathOnce.Do(func() {
		binaryName := "ffmpeg"
		if runtime.GOOS == "windows" {
			binaryName = "ffmpeg.exe"
		}

		// Check same directory as current executable
		if exe, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exe), binaryName)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				cachedFFmpegPath = candidate
				return
			}
		}

		cachedFFmpegPath = binaryName
	})
	return cachedFFmpegPath
}

// buildInputArgs returns ffmpeg input arguments with RTSP transport settings.
// RTSPTransport can be "tcp", "udp", or "" (empty for FFmpeg's default).
func buildInputArgs(streamURL string, rtspTransport string) []string {
	args := []string{
		// Ignore decoding errors (e.g., HEVC "Could not find ref with POC 0")
		// that occur when starting mid-stream without reference frames.
		"-err_detect", "ignore_err",
		// Discard corrupt packets and generate missing PTS values
		"-fflags", "+discardcorrupt+genpts",
	}
	if rtspTransport != "" {
		args = append(args, "-rtsp_transport", rtspTransport)
	}
	args = append(args, "-i", streamURL)
	return args
}

// CapImgBase64 captures a single JPEG frame from streamURL and returns a data URI and temp file path.
// Parameters:
//   - seek: seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration in seconds for the entire operation (context timeout)
//   - rtspTransport: RTSP transport protocol ("tcp", "udp", or "" for default)
//
// Note: The caller is responsible for cleaning up the temp file after use.
// If the file is stored in MediaStore, it will be cleaned up when the scope is released.
// If not stored, the caller should delete it after reading.
func CapImgBase64(ctx context.Context, streamURL string, seek int, end int, timeout int, rtspTransport string) (dataURI string, filePath string, err error) {
	imgDir := hcconfig.WorkspaceImgDir()
	if mkErr := os.MkdirAll(imgDir, 0o750); mkErr != nil {
		imgDir = os.TempDir()
	}
	tmpFile := filepath.Join(imgDir, fmt.Sprintf("homeclaw_frame_%s.jpg", GenerateUUID()))

	if err := capImg2File(ctx, streamURL, seek, end, timeout, tmpFile, rtspTransport); err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		os.Remove(tmpFile) //nolint:errcheck
		return "", "", fmt.Errorf("failed to read captured frame: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return "data:image/jpeg;base64," + encoded, tmpFile, nil
}

// capImg captures a single JPEG frame from streamURL and returns the raw bytes.
// Parameters:
//   - seek: seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration in seconds for the entire operation (context timeout)
//   - fileName: output file name (used for temp file naming)
//   - rtspTransport: RTSP transport protocol ("tcp", "udp", or "")
func capImg(ctx context.Context, streamURL string, seek int, end int, timeout int, fileName string, rtspTransport string) ([]byte, string, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fileName)

	defer os.Remove(tmpFile) //nolint:errcheck

	if err := capImg2File(ctx, streamURL, seek, end, timeout, tmpFile, rtspTransport); err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read captured frame: %w", err)
	}
	return data, tmpFile, nil
}

// capImg2File captures a single JPEG frame from streamURL and saves it to the specified file.
// Parameters:
//   - seek: seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration in seconds for the entire operation (context timeout)
//   - fileName: output file path where the frame will be saved
//   - rtspTransport: RTSP transport protocol ("tcp", "udp", or "")
func capImg2File(ctx context.Context, streamURL string, seek int, end int, timeout int, fileName string, rtspTransport string) error {
	inputArgs := buildInputArgs(streamURL, rtspTransport)

	// Build output args
	outputArgs := []string{
		"-t", fmt.Sprintf("%d", end),
	}
	if seek > 0 {
		outputArgs = append(outputArgs, "-ss", fmt.Sprintf("%d", seek))
	}
	outputArgs = append(outputArgs,
		"-frames:v", "1",
		"-f", "image2",
		"-y", fileName,
	)

	args := append(inputArgs, outputArgs...)

	if err := runFFmpegWithTimeout(ctx, args, timeout); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("frame capture cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("ffmpeg frame capture failed: %w", err)
	}

	// Verify the file was created and is not empty
	data, err := os.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("failed to read captured frame: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("captured frame is empty")
	}
	return nil
}

// runFFmpegWithTimeout runs ffmpeg with the given arguments, starts it, and waits for
// completion while honouring ctx cancellation. When ctx is cancelled the
// ffmpeg process is killed immediately.
// A timeout is enforced to prevent indefinite hangs.
// stderr is captured and included in the returned error to aid diagnosis.
// timeout: timeout in seconds
func runFFmpegWithTimeout(ctx context.Context, args []string, timeout int) error {
	// Enforce a timeout to prevent hangs when ffmpeg gets stuck
	// (e.g., TCP connection waiting for unreachable RTSP server).
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	ffmpegPath := findFFmpegBinary()
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			stderr := strings.TrimSpace(stderrBuf.String())
			if stderr != "" {
				return fmt.Errorf("%w\nffmpeg stderr: %s", err, stderr)
			}
		}
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done // drain so the goroutine exits cleanly
		return ctx.Err()
	}
}

// CheckFFmpeg verifies that the ffmpeg binary is available on PATH.
// Returns nil if found, or a descriptive error if not.
// Call this at startup or in tool.Execute to surface a clear message
// instead of a cryptic process-launch failure.
func CheckFFmpeg() error {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		// Go 1.19+ returns exec.ErrDot when executable is found in current directory.
		// This is a security feature, but if LookPath returned a path, ffmpeg exists.
		// We accept it if the path is valid (user explicitly placed ffmpeg there).
		if path != "" && errors.Is(err, exec.ErrDot) {
			// ffmpeg found in current directory - this is acceptable
			return nil
		}
		return fmt.Errorf("Must Confirm!ffmpeg binary not found on PATH: %w\nInstall ffmpeg and ensure it is accessible (e.g. add its directory to the system PATH)", err)
	}
	return nil
}
