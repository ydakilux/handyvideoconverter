package encoder

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

var amfQPTable = qualityTable{
	{16, 18, 20, 22}, // high_quality
	{20, 22, 24, 27}, // balanced
	{24, 26, 28, 31}, // space_saver
}

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
	qp := strconv.Itoa(qualityValue(preset, width, amfQPTable))
	return []string{"-rc", "cqp", "-qp_i", qp, "-qp_p", qp, "-quality", amfQuality(preset)}
}

func (e *AmfEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{}
}

func (e *AmfEncoder) IsAvailable(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), TrialEncodeTimeout)
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
