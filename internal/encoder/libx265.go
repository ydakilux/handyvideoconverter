package encoder

import (
	"strconv"
	"strings"
)

var libx265CRFTable = qualityTable{
	{19, 20, 21, 23}, // high_quality
	{23, 25, 27, 30}, // balanced
	{26, 28, 30, 33}, // space_saver
}

// Libx265Encoder implements Encoder for the libx265 (CPU) codec.
type Libx265Encoder struct{}

// NewLibx265Encoder creates a new libx265 encoder instance.
func NewLibx265Encoder() *Libx265Encoder {
	return &Libx265Encoder{}
}

// Name returns the FFmpeg codec name for the CPU-based libx265 encoder.
func (e *Libx265Encoder) Name() string {
	return "libx265"
}

// QualityArgs returns libx265 quality arguments (-crf and -preset) for the given preset and resolution.
func (e *Libx265Encoder) QualityArgs(preset string, width int) []string {
	crf := strconv.Itoa(qualityValue(preset, width, libx265CRFTable))
	return []string{"-crf", crf, "-preset", libx265Preset(preset)}
}

// DeviceArgs returns an empty slice; CPU encoding has no device selection.
func (e *Libx265Encoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

// IsAvailable always returns true because libx265 is a software encoder.
func (e *Libx265Encoder) IsAvailable(ffmpegPath string) bool {
	return true
}

// ParseError always returns false; CPU encoding does not produce GPU-specific errors.
func (e *Libx265Encoder) ParseError(stderr string) (bool, string) {
	return false, ""
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
