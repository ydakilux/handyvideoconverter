package ffmpeg

import (
	"strings"
	"testing"
)

func TestBuildArgsLibx265MP4(t *testing.T) {
	args := BuildArgs("/input.mp4", "/output.mp4", "27", ".mp4", "libx265")

	assertContains(t, args, "-hide_banner")
	assertContains(t, args, "-y")
	assertContains(t, args, "-nostats")
	assertContainsSequence(t, args, "-progress", "pipe:1")
	assertContainsSequence(t, args, "-i", "/input.mp4")
	assertContainsSequence(t, args, "-c:v", "libx265")
	assertContainsSequence(t, args, "-crf", "27")
	assertContainsSequence(t, args, "-preset", "medium")
	assertContainsSequence(t, args, "-c:a", "aac")
	assertContainsSequence(t, args, "-movflags", "+faststart")

	if args[len(args)-1] != "/output.mp4" {
		t.Errorf("last arg should be output path, got %q", args[len(args)-1])
	}

	assertNotContains(t, args, "-cq")
}

func TestBuildArgsNvencMP4(t *testing.T) {
	args := BuildArgs("/input.mp4", "/output.mp4", "30", ".mp4", "hevc_nvenc")

	assertContainsSequence(t, args, "-c:v", "hevc_nvenc")
	assertContainsSequence(t, args, "-cq", "30")
	assertContainsSequence(t, args, "-preset", "p5")
	assertContainsSequence(t, args, "-c:a", "aac")
	assertContainsSequence(t, args, "-movflags", "+faststart")
	assertNotContains(t, args, "-crf")

	if args[len(args)-1] != "/output.mp4" {
		t.Errorf("last arg should be output path, got %q", args[len(args)-1])
	}
}

func TestBuildArgsMKV(t *testing.T) {
	args := BuildArgs("/input.mkv", "/output.mkv", "25", ".mkv", "libx265")

	assertContainsSequence(t, args, "-c:v", "libx265")
	assertContainsSequence(t, args, "-crf", "25")
	assertContainsSequence(t, args, "-preset", "medium")
	assertContainsSequence(t, args, "-c:a", "copy")
	assertContainsSequence(t, args, "-c:s", "copy")
	assertContains(t, args, "-map")
	assertContains(t, args, "-map_metadata")
	assertContains(t, args, "-map_chapters")
	assertNotContains(t, args, "-movflags")
	assertNotContains(t, args, "aac")

	if args[len(args)-1] != "/output.mkv" {
		t.Errorf("last arg should be output path, got %q", args[len(args)-1])
	}
}

func TestBuildArgsNvencMKV(t *testing.T) {
	args := BuildArgs("/input.mkv", "/output.mkv", "28", ".mkv", "hevc_nvenc")

	assertContainsSequence(t, args, "-c:v", "hevc_nvenc")
	assertContainsSequence(t, args, "-cq", "28")
	assertContainsSequence(t, args, "-preset", "p5")
	assertContainsSequence(t, args, "-c:a", "copy")
	assertContainsSequence(t, args, "-c:s", "copy")
	assertContains(t, args, "-map")
	assertNotContains(t, args, "-movflags")
	assertNotContains(t, args, "aac")
	assertNotContains(t, args, "-crf")
}

func TestBuildArgsDefaultEncoder(t *testing.T) {
	args := BuildArgs("/input.mp4", "/output.mp4", "22", ".mp4", "libx264")

	assertContainsSequence(t, args, "-c:v", "libx264")
	assertContainsSequence(t, args, "-crf", "22")
	assertNotContainsSequence(t, args, "-preset", "medium")
	assertNotContains(t, args, "-cq")
}

func TestIsHEVC(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		codecID  string
		expected bool
	}{
		{"HEVC format", "HEVC", "", true},
		{"hevc lowercase", "hevc", "", true},
		{"HVC1 codecID", "", "hvc1", true},
		{"HVC1 upper", "", "HVC1", true},
		{"Both HEVC", "HEVC", "HVC1", true},
		{"H264 format", "H264", "avc1", false},
		{"AVC format", "AVC", "V_MPEG4/ISO/AVC", false},
		{"Empty", "", "", false},
		{"Partial match HEVC in format", "Some HEVC Thing", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHEVC(tt.format, tt.codecID)
			if got != tt.expected {
				t.Errorf("IsHEVC(%q, %q) = %v, want %v", tt.format, tt.codecID, got, tt.expected)
			}
		})
	}
}

func assertContainsSequence(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return
		}
	}
	t.Errorf("args missing consecutive sequence %q %q\nargs: %s", key, value, strings.Join(args, " "))
}

func assertNotContainsSequence(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			t.Errorf("args should NOT contain consecutive sequence %q %q\nargs: %s", key, value, strings.Join(args, " "))
			return
		}
	}
}

func assertContains(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("args missing %q\nargs: %s", value, strings.Join(args, " "))
}

func assertNotContains(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			t.Errorf("args should NOT contain %q\nargs: %s", value, strings.Join(args, " "))
			return
		}
	}
}
