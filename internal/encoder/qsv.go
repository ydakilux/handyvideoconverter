package encoder

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

func (e *QsvEncoder) Name() string {
	return "hevc_qsv"
}

func (e *QsvEncoder) QualityArgs(preset string, width int) []string {
	gq := strconv.Itoa(qualityValue(preset, width, qsvGQTable))
	return []string{"-global_quality", gq, "-preset", qsvPreset(preset)}
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
