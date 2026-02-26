package detect

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type GPUInfo struct {
	Name          string
	Encoder       string
	DeviceIndex   int
	Available     bool
	TrialEncodeMs int64
}

type DetectionResult struct {
	Available   []GPUInfo
	Preferred   GPUInfo
	CPUFallback GPUInfo
}

var knownEncoders = []struct {
	encoder     string
	name        string
	deviceIndex int
}{
	{"hevc_nvenc", "NVIDIA NVENC", 0},
	{"hevc_amf", "AMD AMF", 0},
	{"hevc_qsv", "Intel QSV", 0},
}

func PriorityOrder() []string {
	return []string{"hevc_nvenc", "hevc_amf", "hevc_qsv", "libx265"}
}

func SelectBest(result *DetectionResult) GPUInfo {
	priority := PriorityOrder()
	lookup := make(map[string]GPUInfo, len(result.Available))
	for _, info := range result.Available {
		lookup[info.Encoder] = info
	}

	for _, enc := range priority {
		if info, ok := lookup[enc]; ok && info.Available {
			return info
		}
	}

	return result.CPUFallback
}

func DetectEncoders(ffmpegPath string, logger *logrus.Logger) (*DetectionResult, error) {
	cpuFallback := GPUInfo{
		Name:        "CPU x265",
		Encoder:     "libx265",
		DeviceIndex: -1,
		Available:   true,
	}

	candidates := listEncoders(ffmpegPath, logger)

	var available []GPUInfo
	for _, known := range knownEncoders {
		info := GPUInfo{
			Name:        known.name,
			Encoder:     known.encoder,
			DeviceIndex: known.deviceIndex,
			Available:   false,
		}

		if candidates[known.encoder] {
			probed := trialEncode(ffmpegPath, known.encoder, logger)
			info.Available = probed.Available
			info.TrialEncodeMs = probed.TrialEncodeMs
		}

		available = append(available, info)
	}
	available = append(available, cpuFallback)

	result := &DetectionResult{
		Available:   available,
		CPUFallback: cpuFallback,
	}
	result.Preferred = SelectBest(result)

	return result, nil
}

func listEncoders(ffmpegPath string, logger *logrus.Logger) map[string]bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, "-encoders", "-hide_banner")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debugf("Failed to list encoders: %v", err)
		return make(map[string]bool)
	}

	return parseEncoderList(string(output))
}

func parseEncoderList(output string) map[string]bool {
	targets := map[string]bool{
		"hevc_nvenc": false,
		"hevc_amf":   false,
		"hevc_qsv":   false,
		"libx265":    false,
	}

	found := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		for target := range targets {
			if strings.Contains(trimmed, target) {
				found[target] = true
			}
		}
	}

	return found
}

func trialEncode(ffmpegPath string, encoder string, logger *logrus.Logger) GPUInfo {
	info := GPUInfo{
		Encoder:   encoder,
		Available: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "lavfi",
		"-i", "color=c=black:s=256x256:d=0.1",
		"-frames:v", "1",
		"-c:v", encoder,
		"-f", "null",
		"-",
	)

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		logger.Debugf("Trial encode failed for %s (%dms): %v — %s", encoder, elapsed, err, string(output))
		return info
	}

	info.Available = true
	info.TrialEncodeMs = elapsed
	logger.Debugf("Trial encode succeeded for %s in %dms", encoder, elapsed)

	return info
}
