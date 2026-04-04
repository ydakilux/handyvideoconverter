package encoder

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

var nvencCQTable = qualityTable{
	{20, 22, 24, 26}, // high_quality
	{24, 26, 28, 30}, // balanced
	{28, 30, 32, 35}, // space_saver
}

// NvencEncoder implements Encoder for the hevc_nvenc (NVIDIA NVENC) codec.
type NvencEncoder struct{}

// NewNvencEncoder creates a new NVENC encoder instance.
func NewNvencEncoder() *NvencEncoder {
	return &NvencEncoder{}
}

func (e *NvencEncoder) Name() string {
	return "hevc_nvenc"
}

func (e *NvencEncoder) QualityArgs(preset string, width int) []string {
	cq := strconv.Itoa(qualityValue(preset, width, nvencCQTable))
	return []string{"-cq", cq, "-preset", nvencPreset(preset)}
}

func (e *NvencEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{"-gpu", strconv.Itoa(gpuIndex)}
}

func (e *NvencEncoder) IsAvailable(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), TrialEncodeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1",
		"-c:v", "hevc_nvenc",
		"-f", "null", "-",
	)
	err := cmd.Run()
	return err == nil
}

func (e *NvencEncoder) ParseError(stderr string) (bool, string) {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "no capable devices found") {
		return true, "NVENC: no capable GPU devices"
	}
	if strings.Contains(lower, "openencodesessionex failed") {
		return true, "NVENC: session limit or memory error"
	}
	if strings.Contains(lower, "initializeencoder failed") {
		return true, "NVENC: encoder initialization failed"
	}
	// Driver too old: FFmpeg built against a newer NVENC SDK than the installed driver.
	// e.g. "Driver does not support the required nvenc API version. Required: 13.0 Found: 12.2"
	if strings.Contains(lower, "does not support the required nvenc api version") ||
		strings.Contains(lower, "minimum required nvidia driver") ||
		strings.Contains(lower, "error while opening encoder") {
		return true, "NVENC: driver too old — update NVIDIA driver to use GPU encoding"
	}
	return false, ""
}

func nvencPreset(preset string) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		return "p7"
	case "space_saver":
		return "p4"
	default:
		return "p5"
	}
}
