package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

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

// ── Integration tests (require ffmpeg/ffprobe in PATH) ─────────────────────

// lookupExe tries "ffmpeg.exe" first (avoids .cmd wrappers on Windows),
// then falls back to the plain name.
func lookupExe(t *testing.T, name string) string {
	t.Helper()
	// On Windows, prefer the .exe over a .cmd wrapper
	if p, err := exec.LookPath(name + ".exe"); err == nil {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not in PATH, skipping integration test", name)
	}
	return p
}

// ffmpegPath returns the ffmpeg executable path, or skips the test.
func ffmpegPath(t *testing.T) string {
	t.Helper()
	return lookupExe(t, "ffmpeg")
}

// ffprobePath returns the ffprobe executable path, or skips the test.
func ffprobePath(t *testing.T) string {
	t.Helper()
	return lookupExe(t, "ffprobe")
}

// makeTestVideo creates a short synthetic video file using ffmpeg's lavfi source.
// Returns the path to the created file; cleanup is handled via t.TempDir.
func makeTestVideo(t *testing.T, ffmpeg string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "test.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "40",
		"-c:a", "aac",
		out,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create test video: %v\n%s", err, out)
	}
	return out
}

func TestGetDurationIntegration(t *testing.T) {
	ffpPath := ffprobePath(t)
	ffmPath := ffmpegPath(t)
	videoPath := makeTestVideo(t, ffmPath)

	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	dur := GetDuration(videoPath, ffpPath, logger)
	if dur <= 0 {
		t.Errorf("GetDuration should return positive value, got %f", dur)
	}
	// Expect roughly 2 seconds (±1s tolerance for encoding)
	if dur < 1.0 || dur > 4.0 {
		t.Errorf("GetDuration returned unexpected value: %f (expected ~2s)", dur)
	}
}

func TestGetDurationEmptyExe(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	dur := GetDuration("video.mp4", "", logger)
	if dur != 0.0 {
		t.Errorf("GetDuration with empty exe should return 0, got %f", dur)
	}
}

func TestGetDurationMissingFile(t *testing.T) {
	ffpPath := ffprobePath(t)
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	dur := GetDuration("/nonexistent/file.mp4", ffpPath, logger)
	if dur != 0.0 {
		t.Errorf("GetDuration on missing file should return 0, got %f", dur)
	}
}

func TestGetMediaInfoIntegration(t *testing.T) {
	ffpPath := ffprobePath(t)
	ffmPath := ffmpegPath(t)
	videoPath := makeTestVideo(t, ffmPath)

	info, err := GetMediaInfo(videoPath, ffpPath)
	if err != nil {
		t.Fatalf("GetMediaInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("GetMediaInfo returned nil info")
	}
	if info.Width != 320 {
		t.Errorf("Width = %d, want 320", info.Width)
	}
	if info.Height != 240 {
		t.Errorf("Height = %d, want 240", info.Height)
	}
	if info.Format == "" {
		t.Error("Format should be non-empty")
	}
}

func TestGetMediaInfoMissingFile(t *testing.T) {
	ffpPath := ffprobePath(t)
	_, err := GetMediaInfo("/nonexistent/video.mp4", ffpPath)
	if err == nil {
		t.Error("GetMediaInfo on missing file should return an error")
	}
}

func TestRunIntegration(t *testing.T) {
	ffmPath := ffmpegPath(t)
	srcPath := makeTestVideo(t, ffmPath)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.mp4")

	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-progress", "pipe:1",
		"-i", srcPath,
		"-c:v", "libx264", "-crf", "40", "-preset", "ultrafast",
		"-c:a", "aac", "-movflags", "+faststart",
		outPath,
	}
	var lastPct int
	progressCalled := false
	rc, _ := Run(context.Background(), ffmPath, args, srcPath, 2.0, logger, func(pct int) {
		progressCalled = true
		lastPct = pct
	}, nil)

	if rc != 0 {
		t.Errorf("Run returned non-zero exit code: %d", rc)
	}
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Error("output file should exist after successful Run")
	}
	_ = progressCalled
	_ = lastPct
}

func TestRunBadArgs(t *testing.T) {
	ffmPath := ffmpegPath(t)
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	rc, stderr := Run(context.Background(), ffmPath, []string{"-invalid_flag_xyz"}, "", 0, logger, nil, nil)
	if rc == 0 {
		t.Error("Run with bad args should return non-zero exit code")
	}
	if stderr == "" {
		t.Error("Run with bad args should return non-empty stderr")
	}
}
