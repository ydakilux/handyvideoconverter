package encoder

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
	cq := nvencCQ(preset, width)
	nvPreset := nvencPreset(preset)
	return []string{"-cq", cq, "-preset", nvPreset}
}

func (e *NvencEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{"-hwaccel_device", strconv.Itoa(gpuIndex)}
}

func (e *NvencEncoder) IsAvailable(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	if strings.Contains(stderr, "No capable devices found") {
		return true, "NVENC: no capable GPU devices"
	}
	if strings.Contains(stderr, "OpenEncodeSessionEx failed") {
		return true, "NVENC: session limit or memory error"
	}
	if strings.Contains(stderr, "InitializeEncoder failed") {
		return true, "NVENC: encoder initialization failed"
	}
	return false, ""
}

func nvencCQ(preset string, width int) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		if width <= 1024 {
			return "20"
		} else if width <= 1280 {
			return "22"
		} else if width <= 1920 {
			return "24"
		}
		return "26"

	case "space_saver":
		if width <= 1024 {
			return "28"
		} else if width <= 1280 {
			return "30"
		} else if width <= 1920 {
			return "32"
		}
		return "35"

	default: // "balanced" and any unknown preset
		if width <= 1024 {
			return "24"
		} else if width <= 1280 {
			return "26"
		} else if width <= 1920 {
			return "28"
		}
		return "30"
	}
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
