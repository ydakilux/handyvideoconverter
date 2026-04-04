// Package encoder defines the Encoder interface and provides implementations
// for various video encoders (CPU and GPU). Each encoder knows how to produce
// the correct FFmpeg quality/device arguments for its codec.
package encoder

import (
	"context"
	"os/exec"
	"time"
)

// TrialEncodeTimeout is the maximum time allowed for a trial encode when probing encoder availability.
const TrialEncodeTimeout = 10 * time.Second

// trialEncode runs a minimal FFmpeg encode (one frame, 256×256 black) using the
// given codec to verify that the encoder is available and functional. Returns
// true only when FFmpeg exits cleanly.
func trialEncode(ffmpegPath, codec string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), TrialEncodeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1",
		"-c:v", codec,
		"-f", "null", "-",
	)
	return cmd.Run() == nil
}

// Encoder abstracts a video encoder's FFmpeg argument generation and availability probing.
type Encoder interface {
	// Name returns the FFmpeg codec name, e.g. "libx265", "hevc_nvenc".
	Name() string

	// QualityArgs returns the full quality-related FFmpeg arg slice for the
	// given quality preset ("balanced", "high_quality", "space_saver") and
	// video width in pixels. The returned slice is ready to append to the
	// FFmpeg command, e.g. ["-crf", "27", "-preset", "medium"].
	QualityArgs(preset string, width int) []string

	// DeviceArgs returns device-selection FFmpeg args for multi-GPU setups.
	// CPU encoders return an empty slice.
	DeviceArgs(gpuIndex int) []string

	// IsAvailable performs a lightweight check (e.g. trial encode) to
	// determine whether the encoder can be used with the given FFmpeg binary.
	// CPU encoders always return true.
	IsAvailable(ffmpegPath string) bool

	// ParseError inspects FFmpeg stderr output and reports whether the
	// error is GPU-specific. CPU encoders always return (false, "").
	ParseError(stderr string) (isGPUError bool, msg string)
}
