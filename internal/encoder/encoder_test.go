package encoder

import (
	"reflect"
	"testing"
)

func TestLibx265Name(t *testing.T) {
	enc := NewLibx265Encoder()
	if got := enc.Name(); got != "libx265" {
		t.Errorf("Name() = %q, want %q", got, "libx265")
	}
}

func TestLibx265QualityArgs(t *testing.T) {
	enc := NewLibx265Encoder()

	tests := []struct {
		name   string
		preset string
		width  int
		want   []string
	}{
		// balanced preset (default)
		{"balanced_sd", "balanced", 800, []string{"-crf", "23", "-preset", "medium"}},
		{"balanced_720p", "balanced", 1280, []string{"-crf", "25", "-preset", "medium"}},
		{"balanced_1080p", "balanced", 1920, []string{"-crf", "27", "-preset", "medium"}},
		{"balanced_4k", "balanced", 3840, []string{"-crf", "30", "-preset", "medium"}},

		// high_quality preset
		{"hq_sd", "high_quality", 1024, []string{"-crf", "19", "-preset", "slow"}},
		{"hq_720p", "high_quality", 1280, []string{"-crf", "20", "-preset", "slow"}},
		{"hq_1080p", "high_quality", 1920, []string{"-crf", "21", "-preset", "slow"}},
		{"hq_4k", "high_quality", 3840, []string{"-crf", "23", "-preset", "slow"}},

		// space_saver preset
		{"ss_sd", "space_saver", 640, []string{"-crf", "26", "-preset", "faster"}},
		{"ss_720p", "space_saver", 1280, []string{"-crf", "28", "-preset", "faster"}},
		{"ss_1080p", "space_saver", 1920, []string{"-crf", "30", "-preset", "faster"}},
		{"ss_4k", "space_saver", 2560, []string{"-crf", "33", "-preset", "faster"}},

		// unknown preset falls back to balanced
		{"unknown_1080p", "custom", 1920, []string{"-crf", "27", "-preset", "medium"}},
		{"empty_1080p", "", 1920, []string{"-crf", "27", "-preset", "medium"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enc.QualityArgs(tt.preset, tt.width)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("QualityArgs(%q, %d) = %v, want %v", tt.preset, tt.width, got, tt.want)
			}
		})
	}
}

func TestLibx265DeviceArgs(t *testing.T) {
	enc := NewLibx265Encoder()
	got := enc.DeviceArgs(0)
	if len(got) != 0 {
		t.Errorf("DeviceArgs(0) = %v, want empty slice", got)
	}
}

func TestLibx265IsAvailable(t *testing.T) {
	enc := NewLibx265Encoder()
	if !enc.IsAvailable("") {
		t.Error("IsAvailable() = false, want true")
	}
	if !enc.IsAvailable("/nonexistent/ffmpeg") {
		t.Error("IsAvailable(/nonexistent/ffmpeg) = false, want true")
	}
}

func TestLibx265ParseError(t *testing.T) {
	enc := NewLibx265Encoder()
	isGPU, msg := enc.ParseError("some random stderr output")
	if isGPU {
		t.Error("ParseError() isGPUError = true, want false")
	}
	if msg != "" {
		t.Errorf("ParseError() msg = %q, want empty string", msg)
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	enc := NewLibx265Encoder()

	reg.Register(enc)

	got, ok := reg.Get("libx265")
	if !ok {
		t.Fatal("Get(libx265) returned false, want true")
	}
	if got.Name() != "libx265" {
		t.Errorf("Get(libx265).Name() = %q, want %q", got.Name(), "libx265")
	}
}

func TestRegistryGetNonExistent(t *testing.T) {
	reg := NewRegistry()

	_, ok := reg.Get("hevc_nvenc")
	if ok {
		t.Error("Get(hevc_nvenc) returned true for empty registry, want false")
	}
}

func TestRegistryAll(t *testing.T) {
	reg := NewRegistry()
	enc := NewLibx265Encoder()
	reg.Register(enc)

	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d encoders, want 1", len(all))
	}
	if all[0].Name() != "libx265" {
		t.Errorf("All()[0].Name() = %q, want %q", all[0].Name(), "libx265")
	}
}

func TestLibx265ImplementsEncoder(t *testing.T) {
	var _ Encoder = (*Libx265Encoder)(nil)
}
