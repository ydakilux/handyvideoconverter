package encoder

import "strings"

// Libx265Encoder implements Encoder for the libx265 (CPU) codec.
type Libx265Encoder struct{}

// NewLibx265Encoder creates a new libx265 encoder instance.
func NewLibx265Encoder() *Libx265Encoder {
	return &Libx265Encoder{}
}

func (e *Libx265Encoder) Name() string {
	return "libx265"
}

func (e *Libx265Encoder) QualityArgs(preset string, width int) []string {
	crf := libx265CRF(preset, width)
	x265Preset := libx265Preset(preset)
	return []string{"-crf", crf, "-preset", x265Preset}
}

func (e *Libx265Encoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

func (e *Libx265Encoder) IsAvailable(ffmpegPath string) bool {
	return true
}

func (e *Libx265Encoder) ParseError(stderr string) (bool, string) {
	return false, ""
}

func libx265CRF(preset string, width int) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		if width <= 1024 {
			return "19"
		} else if width <= 1280 {
			return "20"
		} else if width <= 1920 {
			return "21"
		}
		return "23"

	case "space_saver":
		if width <= 1024 {
			return "26"
		} else if width <= 1280 {
			return "28"
		} else if width <= 1920 {
			return "30"
		}
		return "33"

	default: // "balanced" and any unknown preset
		if width <= 1024 {
			return "23"
		} else if width <= 1280 {
			return "25"
		} else if width <= 1920 {
			return "27"
		}
		return "30"
	}
}

func libx265Preset(preset string) string {
	switch strings.ToLower(preset) {
	case "high_quality":
		return "slow"
	case "space_saver":
		return "faster"
	default:
		return "medium"
	}
}
