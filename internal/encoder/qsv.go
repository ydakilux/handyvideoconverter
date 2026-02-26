package encoder

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// QsvEncoder implements Encoder for the hevc_qsv (Intel Quick Sync Video) codec.
type QsvEncoder struct{}

// NewQsvEncoder creates a new QSV encoder instance.
func NewQsvEncoder() *QsvEncoder {
	return &QsvEncoder{}
}

func (e *QsvEncoder) Name() string {
	return "hevc_qsv"
}

func (e *QsvEncoder) QualityArgs(preset string, width int) []string {
	gq := qsvGlobalQuality(preset, width)
	qsvPresetName := qsvPreset(preset)
	return []string{"-global_quality", gq, "-preset", qsvPresetName}
}

func (e *QsvEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

func (e *QsvEncoder) IsAvailable(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1",
		"-c:v", "hevc_qsv",
		"-f", "null", "-",
	)
	return cmd.Run() == nil
}

func (e *QsvEncoder) ParseError(stderr string) (bool, string) {
	if strings.Contains(stderr, "Error initializing an MFX session") {
		return true, "QSV: MFX session initialization failed"
	}
	if strings.Contains(stderr, "Error during encoding") && strings.Contains(strings.ToLower(stderr), "qsv") {
		return true, "QSV: encoding error"
	}
	return false, ""
}

func qsvGlobalQuality(preset string, width int) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		if width <= 1024 {
			return "17"
		} else if width <= 1280 {
			return "19"
		} else if width <= 1920 {
			return "21"
		}
		return "23"

	case "space_saver":
		if width <= 1024 {
			return "25"
		} else if width <= 1280 {
			return "27"
		} else if width <= 1920 {
			return "30"
		}
		return "33"

	default: // "balanced" and any unknown preset
		if width <= 1024 {
			return "21"
		} else if width <= 1280 {
			return "23"
		} else if width <= 1920 {
			return "25"
		}
		return "28"
	}
}

func qsvPreset(preset string) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		return "veryslow"
	case "space_saver":
		return "faster"
	default:
		return "medium"
	}
}