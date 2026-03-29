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
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// rtspConnTimeoutSec is the maximum number of seconds ffmpeg is allowed to run
// when capturing a single frame. Passed via -t (max duration) which is universally
// supported across all ffmpeg builds. For a single-frame capture ffmpeg exits as
// soon as the frame is written, so this only fires when the stream is unreachable
// or stalled, preventing indefinite hangs.
const rtspConnTimeoutSec = 4 // 4 seconds

// ffmpegTimeout is the maximum duration allowed for ffmpeg execution.
// This ensures the operation never hangs indefinitely even if ffmpeg gets stuck.
const ffmpegTimeout = 6 * time.Second

// FrameGrabber captures a single frame from an RTSP (or any FFmpeg-compatible) stream.
type FrameGrabber struct {
	// RTSPTransport forces a specific RTSP transport protocol ("tcp", "udp", "").
	// Leave empty to use FFmpeg's default.
	RTSPTransport string

	// SeekSeconds is the number of seconds to seek into the stream before
	// capturing a frame. Default is 3 to skip the black opening frames that
	// many RTSP cameras emit at the start of a connection.
	SeekSeconds float64
}

// NewFrameGrabber creates a new FrameGrabber with sensible defaults.
// SeekSeconds defaults to 3 to avoid the initial black frame common on RTSP streams.
func NewFrameGrabber() *FrameGrabber {
	return &FrameGrabber{
		RTSPTransport: "tcp",
		SeekSeconds:   3,
	}
}

// buildInputOpts returns ffmpeg input KwArgs populated from the grabber settings.
// NOTE: -ss is intentionally NOT placed here (input side) because many RTSP
// servers (e.g. Xiaomi cameras) do not support seeking and will return an error.
// Instead, -ss is placed on the output side so ffmpeg decodes and discards
// frames for SeekSeconds before capturing, which works universally.
func (g *FrameGrabber) buildInputOpts() ffmpeg.KwArgs {
	opts := ffmpeg.KwArgs{
		// Ignore decoding errors (e.g., HEVC "Could not find ref with POC 0")
		// that occur when starting mid-stream without reference frames.
		"err_detect": "ignore_err",
		// Discard corrupt packets and generate missing PTS values
		"fflags": "+discardcorrupt+genpts",
	}
	if g.RTSPTransport != "" {
		opts["rtsp_transport"] = g.RTSPTransport
	}
	return opts
}

// buildOutputOpts returns ffmpeg output KwArgs for a single-frame JPEG capture.
// -ss on the output side causes ffmpeg to decode and discard frames for
// SeekSeconds before writing the captured frame, avoiding black/green
// initialization frames without requiring server-side seek support.
// -t caps the total run time so ffmpeg never hangs indefinitely on a stalled stream.
func (g *FrameGrabber) buildOutputOpts(extra ffmpeg.KwArgs) ffmpeg.KwArgs {
	opts := ffmpeg.KwArgs{
		"t": rtspConnTimeoutSec,
	}
	if g.SeekSeconds > 0 {
		opts["ss"] = g.SeekSeconds
	}
	for k, v := range extra {
		opts[k] = v
	}
	return opts
}

// runWithContext compiles a Stream to an *exec.Cmd, starts it, and waits for
// completion while honouring ctx cancellation. When ctx is cancelled the
// ffmpeg process is killed immediately.
// An internal timeout of ffmpegTimeout is enforced to prevent indefinite hangs.
// stderr is captured and included in the returned error to aid diagnosis.
func runWithContext(ctx context.Context, stream *ffmpeg.Stream) error {
	// Enforce an internal timeout to prevent hangs when ffmpeg gets stuck
	// (e.g., TCP connection waiting for unreachable RTSP server).
	ctx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()

	cmd := stream.Compile()

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

// GrabFrameBytes captures a single JPEG frame from streamURL and returns the
// raw bytes. The capture is aborted if ctx is cancelled before completion.
func (g *FrameGrabber) GrabFrameBytes(ctx context.Context, streamURL string) ([]byte, error) {
	// Write to a temp file because ffmpeg-go's pipe output does not always
	// flush cleanly for single-frame JPEG captures.
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("homeclaw_frame_%d.jpg", uniqueID()))

	defer os.Remove(tmpFile) //nolint:errcheck

	inputOpts := g.buildInputOpts()

	// Build the ffmpeg command:
	//   ffmpeg [-stimeout N] [-rtsp_transport tcp] -i <url>
	//          [-ss N] -frames:v 1 -f image2 <tmpfile>
	// -ss is on the output side so ffmpeg decodes/discards frames instead of
	// seeking, which works with RTSP servers that do not support seeking.
	stream := ffmpeg.Input(streamURL, inputOpts).
		Output(tmpFile, g.buildOutputOpts(ffmpeg.KwArgs{
			"frames:v": 1,
			"f":        "image2",
		})).
		OverWriteOutput()

	if err := runWithContext(ctx, stream); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("frame capture cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffmpeg frame capture failed: %w", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read captured frame: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("captured frame is empty")
	}
	return data, nil
}

// GrabFrameToBuffer captures a single JPEG frame directly into a bytes.Buffer
// using ffmpeg's pipe output. This avoids a temp-file round trip when the
// system supports it.
func (g *FrameGrabber) GrabFrameToBuffer(ctx context.Context, streamURL string) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}

	inputOpts := g.buildInputOpts()

	stream := ffmpeg.Input(streamURL, inputOpts).
		Output("pipe:", g.buildOutputOpts(ffmpeg.KwArgs{
			"frames:v": 1,
			"format":   "image2",
			"vcodec":   "mjpeg",
		})).
		WithOutput(buf)

	if err := runWithContext(ctx, stream); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("frame capture cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffmpeg frame capture failed: %w", err)
	}

	if buf.Len() == 0 {
		return nil, fmt.Errorf("captured frame buffer is empty")
	}
	return buf, nil
}

// GrabFrameAsDataURI captures a single JPEG frame and returns a base64-encoded
// data URI string suitable for embedding in an LLM Message.Media field:
//
//	"data:image/jpeg;base64,<base64data>"
func (g *FrameGrabber) GrabFrameAsDataURI(ctx context.Context, streamURL string) (string, error) {
	raw, err := g.GrabFrameBytes(ctx, streamURL)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	return "data:image/jpeg;base64," + encoded, nil
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
		return fmt.Errorf("ffmpeg binary not found on PATH: %w\nInstall ffmpeg and ensure it is accessible (e.g. add its directory to the system PATH)", err)
	}
	return nil
}
