package encoder

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// AmfEncoder implements Encoder for the AMD AMF (hevc_amf) codec.
type AmfEncoder struct{}

// NewAmfEncoder creates a new AMD AMF encoder instance.
func NewAmfEncoder() *AmfEncoder {
	return &AmfEncoder{}
}

func (e *AmfEncoder) Name() string {
	return "hevc_amf"
}

func (e *AmfEncoder) QualityArgs(preset string, width int) []string {
	qp := amfQP(preset, width)
	qualityPreset := amfQuality(preset)
	return []string{"-rc", "cqp", "-qp_i", qp, "-qp_p", qp, "-quality", qualityPreset}
}

func (e *AmfEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

func (e *AmfEncoder) IsAvailable(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1", "-c:v", "hevc_amf", "-f", "null", "-",
	)
	return cmd.Run() == nil
}

func (e *AmfEncoder) ParseError(stderr string) (bool, string) {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "encoder creation error") {
		return true, "AMF: encoder creation failed"
	}
	if strings.Contains(lower, "amf") && strings.Contains(lower, "error") {
		return true, "AMF: encoding error"
	}
	return false, ""
}

func amfQP(preset string, width int) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		if width <= 1024 {
			return "16"
		} else if width <= 1280 {
			return "18"
		} else if width <= 1920 {
			return "20"
		}
		return "22"

	case "space_saver":
		if width <= 1024 {
			return "24"
		} else if width <= 1280 {
			return "26"
		} else if width <= 1920 {
			return "28"
		}
		return "31"

	default: // "balanced" and any unknown preset
		if width <= 1024 {
			return "20"
		} else if width <= 1280 {
			return "22"
		} else if width <= 1920 {
			return "24"
		}
		return "27"
	}
}

func amfQuality(preset string) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		return "quality"
	case "space_saver":
		return "speed"
	default:
		return "balanced"
	}
}