// Package video provides utilities for capturing frames from video streams.
package video

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
	"time"
)

// FFmpegUtil provides utilities for capturing frames from video streams using FFmpeg.
type FFmpegUtil struct {
	// RTSPTransport forces a specific RTSP transport protocol ("tcp", "udp", "").
	// Leave empty to use FFmpeg's default.
	RTSPTransport string
}

// NewFFmpegUtil creates a new FFmpegUtil with sensible defaults.
func NewFFmpegUtil() *FFmpegUtil {
	return &FFmpegUtil{
		RTSPTransport: "tcp",
	}
}

// findFFmpegBinary locates the ffmpeg executable.
// Search order:
//  1. Same directory as the current executable
//  2. Falls back to "ffmpeg" and relies on $PATH
func findFFmpegBinary() string {
	binaryName := "ffmpeg"
	if runtime.GOOS == "windows" {
		binaryName = "ffmpeg.exe"
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

// buildInputArgs returns ffmpeg input arguments populated from the util settings.
// NOTE: -ss is intentionally NOT placed here (input side) because many RTSP
// servers (e.g. Xiaomi cameras) do not support seeking and will return an error.
// Instead, -ss is placed on the output side so ffmpeg decodes and discards
// frames before capturing, which works universally.
func (u *FFmpegUtil) buildInputArgs(streamURL string) []string {
	args := []string{
		// Ignore decoding errors (e.g., HEVC "Could not find ref with POC 0")
		// that occur when starting mid-stream without reference frames.
		"-err_detect", "ignore_err",
		// Discard corrupt packets and generate missing PTS values
		"-fflags", "+discardcorrupt+genpts",
	}
	if u.RTSPTransport != "" {
		args = append(args, "-rtsp_transport", u.RTSPTransport)
	}
	args = append(args, "-i", streamURL)
	return args
}

// GrabFrameWithPath captures a single JPEG frame and returns both the raw bytes
// and the path to the temp file. The temp file is NOT deleted, so the caller
// can use it for sending to channels. The caller is responsible for cleanup.
func (u *FFmpegUtil) GrabFrameWithPath(ctx context.Context, streamURL string) (dataURI string, filePath string, err error) {
	return u.captureImg2Base64(ctx, streamURL, 3, 4, 6*time.Second)
}

// captureImg2Base64 captures a single JPEG frame from streamURL and returns a data URI and temp file path.
// Parameters:
//   - seek: number of seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration for the entire operation (context timeout)
func (u *FFmpegUtil) captureImg2Base64(ctx context.Context, streamURL string, seek float64, end int, timeout time.Duration) (dataURI string, filePath string, err error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("homeclaw_frame_%d.jpg", uniqueID()))

	if err := u.captureImg2File(ctx, streamURL, seek, end, timeout, tmpFile); err != nil {
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

// captureImg captures a single JPEG frame from streamURL and returns the raw bytes.
// Parameters:
//   - seek: number of seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration for the entire operation (context timeout)
//   - fileName: output file name (used for temp file naming)
func (u *FFmpegUtil) captureImg(ctx context.Context, streamURL string, seek float64, end int, timeout time.Duration, fileName string) ([]byte, string, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fileName)

	defer os.Remove(tmpFile) //nolint:errcheck

	if err := u.captureImg2File(ctx, streamURL, seek, end, timeout, tmpFile); err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read captured frame: %w", err)
	}
	return data, tmpFile, nil
}

// captureImg2File captures a single JPEG frame from streamURL and saves it to the specified file.
// Parameters:
//   - seek: number of seconds to seek into the stream before capturing (0 to disable)
//   - end: max duration in seconds for ffmpeg to run (passed via -t)
//   - timeout: max duration for the entire operation (context timeout)
//   - fileName: output file path where the frame will be saved
func (u *FFmpegUtil) captureImg2File(ctx context.Context, streamURL string, seek float64, end int, timeout time.Duration, fileName string) error {
	inputArgs := u.buildInputArgs(streamURL)

	// Build output args
	outputArgs := []string{
		"-t", fmt.Sprintf("%d", end),
	}
	if seek > 0 {
		outputArgs = append(outputArgs, "-ss", fmt.Sprintf("%.1f", seek))
	}
	outputArgs = append(outputArgs,
		"-frames:v", "1",
		"-f", "image2",
		"-y", fileName,
	)

	args := append(inputArgs, outputArgs...)

	if err := u.runFFmpegWithTimeout(ctx, args, timeout); err != nil {
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
func (u *FFmpegUtil) runFFmpegWithTimeout(ctx context.Context, args []string, timeout time.Duration) error {
	// Enforce a timeout to prevent hangs when ffmpeg gets stuck
	// (e.g., TCP connection waiting for unreachable RTSP server).
	ctx, cancel := context.WithTimeout(ctx, timeout)
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

// uniqueID returns a monotonically increasing integer used for temp file names.
var _idCounter uint64

func uniqueID() uint64 {
	_idCounter++
	return _idCounter
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
