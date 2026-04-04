package encoder

import (
	"strconv"
	"strings"
)

var qsvGQTable = qualityTable{
	{17, 19, 21, 23}, // high_quality
	{21, 23, 25, 28}, // balanced
	{25, 27, 30, 33}, // space_saver
}

// QsvEncoder implements Encoder for the hevc_qsv (Intel Quick Sync Video) codec.
type QsvEncoder struct{}

// NewQsvEncoder creates a new QSV encoder instance.
func NewQsvEncoder() *QsvEncoder {
	return &QsvEncoder{}
}

// Name returns the FFmpeg codec name for Intel Quick Sync Video.
func (e *QsvEncoder) Name() string {
	return "hevc_qsv"
}

// QualityArgs returns QSV quality arguments (-global_quality and -preset) for the given preset and resolution.
func (e *QsvEncoder) QualityArgs(preset string, width int) []string {
	gq := strconv.Itoa(qualityValue(preset, width, qsvGQTable))
	return []string{"-global_quality", gq, "-preset", qsvPreset(preset)}
}

// DeviceArgs returns an empty slice; QSV does not support device selection via FFmpeg.
func (e *QsvEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

// IsAvailable trial-encodes a short clip to verify QSV works with the given FFmpeg binary.
func (e *QsvEncoder) IsAvailable(ffmpegPath string) bool {
	return trialEncode(ffmpegPath, "hevc_qsv")
}

// ParseError inspects FFmpeg stderr for QSV-specific errors and returns a human-readable message.
func (e *QsvEncoder) ParseError(stderr string) (bool, string) {
	if strings.Contains(stderr, "Error initializing an MFX session") {
		return true, "QSV: MFX session initialization failed"
	}
	if strings.Contains(stderr, "Error during encoding") && strings.Contains(strings.ToLower(stderr), "qsv") {
		return true, "QSV: encoding error"
	}
	return false, ""
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
