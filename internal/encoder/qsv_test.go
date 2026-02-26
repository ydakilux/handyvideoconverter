package encoder

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestQsvImplementsEncoder(t *testing.T) {
	var _ Encoder = (*QsvEncoder)(nil)
}

func TestQsvName(t *testing.T) {
	enc := NewQsvEncoder()
	if got := enc.Name(); got != "hevc_qsv" {
		t.Errorf("Name() = %q, want %q", got, "hevc_qsv")
	}
}

func TestQsvQualityArgs(t *testing.T) {
	enc := NewQsvEncoder()

	tests := []struct {
		name   string
		preset string
		width  int
		want   []string
	}{
		// balanced preset (default)
		{"balanced_sd", "balanced", 800, []string{"-global_quality", "21", "-preset", "medium"}},
		{"balanced_720p", "balanced", 1280, []string{"-global_quality", "23", "-preset", "medium"}},
		{"balanced_1080p", "balanced", 1920, []string{"-global_quality", "25", "-preset", "medium"}},
		{"balanced_4k", "balanced", 3840, []string{"-global_quality", "28", "-preset", "medium"}},

		// high_quality preset
		{"hq_sd", "high_quality", 1024, []string{"-global_quality", "17", "-preset", "veryslow"}},
		{"hq_720p", "high_quality", 1280, []string{"-global_quality", "19", "-preset", "veryslow"}},
		{"hq_1080p", "high_quality", 1920, []string{"-global_quality", "21", "-preset", "veryslow"}},
		{"hq_4k", "high_quality", 3840, []string{"-global_quality", "23", "-preset", "veryslow"}},

		// space_saver preset
		{"ss_sd", "space_saver", 640, []string{"-global_quality", "25", "-preset", "faster"}},
		{"ss_720p", "space_saver", 1280, []string{"-global_quality", "27", "-preset", "faster"}},
		{"ss_1080p", "space_saver", 1920, []string{"-global_quality", "30", "-preset", "faster"}},
		{"ss_4k", "space_saver", 2560, []string{"-global_quality", "33", "-preset", "faster"}},

		// unknown preset falls back to balanced
		{"unknown_1080p", "custom", 1920, []string{"-global_quality", "25", "-preset", "medium"}},
		{"empty_1080p", "", 1920, []string{"-global_quality", "25", "-preset", "medium"}},

		// case-insensitive preset matching
		{"uppercase_balanced", "BALANCED", 1920, []string{"-global_quality", "25", "-preset", "medium"}},
		{"mixedcase_hq", "High_Quality", 1920, []string{"-global_quality", "21", "-preset", "veryslow"}},
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

func TestQsvDeviceArgs(t *testing.T) {
	enc := NewQsvEncoder()

	// QSV doesn't support multi-GPU via FFmpeg, always returns empty
	got0 := enc.DeviceArgs(0)
	if len(got0) != 0 {
		t.Errorf("DeviceArgs(0) = %v, want empty slice", got0)
	}

	got1 := enc.DeviceArgs(1)
	if len(got1) != 0 {
		t.Errorf("DeviceArgs(1) = %v, want empty slice", got1)
	}
}

func TestQsvParseError(t *testing.T) {
	enc := NewQsvEncoder()

	tests := []struct {
		name      string
		stderr    string
		wantIsGPU bool
		wantMsg   string
	}{
		{
			"mfx_session_error",
			"Error initializing an MFX session",
			true,
			"QSV: MFX session initialization failed",
		},
		{
			"encoding_error_qsv",
			"Error during encoding qsv stream",
			true,
			"QSV: encoding error",
		},
		{
			"encoding_error_qsv_uppercase",
			"Error during encoding QSV codec failure",
			true,
			"QSV: encoding error",
		},
		{
			"normal_output",
			"frame= 100 fps=50 q=28.0 size= 1024kB time=00:00:04.00",
			false,
			"",
		},
		{
			"empty_stderr",
			"",
			false,
			"",
		},
		{
			"unrelated_error",
			"Error opening input file: No such file or directory",
			false,
			"",
		},
		{
			"encoding_error_without_qsv",
			"Error during encoding some other codec",
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsGPU, gotMsg := enc.ParseError(tt.stderr)
			if gotIsGPU != tt.wantIsGPU {
				t.Errorf("ParseError(%q) isGPUError = %v, want %v", tt.stderr, gotIsGPU, tt.wantIsGPU)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("ParseError(%q) msg = %q, want %q", tt.stderr, gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestQsvIsAvailable(t *testing.T) {
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found in PATH, skipping availability test")
	}

	enc := NewQsvEncoder()
	// Just verify it doesn't panic; actual result depends on hardware.
	_ = enc.IsAvailable("ffmpeg")
}

func TestQsvIsAvailableInvalidPath(t *testing.T) {
	enc := NewQsvEncoder()
	if enc.IsAvailable("/nonexistent/ffmpeg") {
		t.Error("IsAvailable(/nonexistent/ffmpeg) = true, want false")
	}
}