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

func (e *Libx265Encoder) Name() string {
	return "libx265"
}

func (e *Libx265Encoder) QualityArgs(preset string, width int) []string {
	crf := strconv.Itoa(qualityValue(preset, width, libx265CRFTable))
	return []string{"-crf", crf, "-preset", libx265Preset(preset)}
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
