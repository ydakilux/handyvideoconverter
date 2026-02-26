package detect

import (
	"os/exec"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestPriorityOrder(t *testing.T) {
	order := PriorityOrder()

	expected := []string{"hevc_nvenc", "hevc_amf", "hevc_qsv", "libx265"}

	if len(order) != len(expected) {
		t.Fatalf("PriorityOrder() returned %d items, want %d", len(order), len(expected))
	}

	for i, enc := range expected {
		if order[i] != enc {
			t.Errorf("PriorityOrder()[%d] = %q, want %q", i, order[i], enc)
		}
	}
}

func TestSelectBest(t *testing.T) {
	result := &DetectionResult{
		Available: []GPUInfo{
			{Name: "NVIDIA NVENC", Encoder: "hevc_nvenc", DeviceIndex: 0, Available: false},
			{Name: "AMD AMF", Encoder: "hevc_amf", DeviceIndex: 0, Available: true, TrialEncodeMs: 50},
			{Name: "Intel QSV", Encoder: "hevc_qsv", DeviceIndex: 0, Available: false},
			{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
		},
		CPUFallback: GPUInfo{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
	}

	best := SelectBest(result)

	if best.Encoder != "hevc_amf" {
		t.Errorf("SelectBest() = %q, want %q (highest-priority available)", best.Encoder, "hevc_amf")
	}

	cpuOnly := &DetectionResult{
		Available: []GPUInfo{
			{Name: "NVIDIA NVENC", Encoder: "hevc_nvenc", DeviceIndex: 0, Available: false},
			{Name: "AMD AMF", Encoder: "hevc_amf", DeviceIndex: 0, Available: false},
			{Name: "Intel QSV", Encoder: "hevc_qsv", DeviceIndex: 0, Available: false},
			{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
		},
		CPUFallback: GPUInfo{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
	}

	best = SelectBest(cpuOnly)
	if best.Encoder != "libx265" {
		t.Errorf("SelectBest() with no GPU = %q, want %q", best.Encoder, "libx265")
	}

	nvencAvail := &DetectionResult{
		Available: []GPUInfo{
			{Name: "NVIDIA NVENC", Encoder: "hevc_nvenc", DeviceIndex: 0, Available: true, TrialEncodeMs: 30},
			{Name: "AMD AMF", Encoder: "hevc_amf", DeviceIndex: 0, Available: true, TrialEncodeMs: 50},
			{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
		},
		CPUFallback: GPUInfo{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
	}

	best = SelectBest(nvencAvail)
	if best.Encoder != "hevc_nvenc" {
		t.Errorf("SelectBest() with nvenc available = %q, want %q", best.Encoder, "hevc_nvenc")
	}
}

func TestDetectEncoders_ReturnsLibx265Fallback(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		ffmpegPath = "ffmpeg"
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	result, err := DetectEncoders(ffmpegPath, logger)
	if err != nil {
		t.Fatalf("DetectEncoders() error = %v", err)
	}

	if result.CPUFallback.Encoder != "libx265" {
		t.Errorf("CPUFallback.Encoder = %q, want %q", result.CPUFallback.Encoder, "libx265")
	}
	if !result.CPUFallback.Available {
		t.Errorf("CPUFallback.Available = false, want true")
	}

	found := false
	for _, info := range result.Available {
		if info.Encoder == "libx265" {
			found = true
			if !info.Available {
				t.Errorf("libx265 in Available list has Available=false, want true")
			}
			break
		}
	}
	if !found {
		t.Error("libx265 not found in Available list")
	}

	if result.Preferred.Encoder == "" {
		t.Error("Preferred.Encoder is empty")
	}
	if !result.Preferred.Available {
		t.Errorf("Preferred encoder %q has Available=false", result.Preferred.Encoder)
	}
}

func TestTrialEncode_BogusEncoder(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found in PATH, skipping trial encode test")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	info := trialEncode(ffmpegPath, "totally_fake_encoder_xyz", logger)

	if info.Available {
		t.Errorf("trialEncode with bogus encoder returned Available=true, want false")
	}
	if info.Encoder != "totally_fake_encoder_xyz" {
		t.Errorf("trialEncode returned Encoder=%q, want %q", info.Encoder, "totally_fake_encoder_xyz")
	}
}

func TestParseEncoderList(t *testing.T) {
	sampleOutput := ` Encoders:
 ------
 V..... = Video
 A..... = Audio
 S..... = Subtitle
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D libx265              libx265 H.265 / HEVC (codec hevc)
 V....D hevc_nvenc           NVIDIA NVENC hevc encoder (codec hevc)
 V....D hevc_amf             AMD AMF HEVC encoder (codec hevc)
 V....D hevc_qsv             HEVC (Intel Quick Sync Video acceleration) (codec hevc)
 A....D aac                  AAC (Advanced Audio Coding)
 A....D libmp3lame           libmp3lame MP3 (MPEG audio layer 3) (codec mp3)
`

	found := parseEncoderList(sampleOutput)

	expectedEncoders := []string{"hevc_nvenc", "hevc_amf", "hevc_qsv", "libx265"}
	for _, enc := range expectedEncoders {
		if !found[enc] {
			t.Errorf("parseEncoderList did not find %q in output", enc)
		}
	}

	if found["libx264"] {
		t.Error("parseEncoderList should not include libx264")
	}
	if found["aac"] {
		t.Error("parseEncoderList should not include aac")
	}
}
