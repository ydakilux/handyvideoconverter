package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"io"

	"github.com/sirupsen/logrus"

	cfgpkg "video-converter/internal/config"
	"video-converter/internal/converter"
	"video-converter/internal/database"
	"video-converter/internal/fileutil"
	"video-converter/internal/types"
)

// containsExtCompat is a local copy of config.containsExt for use in
// package main_test (which cannot import internal/config).
func containsExtCompat(list []string, ext string) bool {
	upper := strings.ToUpper(ext)
	for _, e := range list {
		if strings.ToUpper(e) == upper {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Test 1: Old-format config (no GPU fields) loads cleanly
// ---------------------------------------------------------------------------

func TestBackwardCompatConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "old_config.json")

	oldJSON := `{
  "seq": {
    "enabled": false,
    "server_url": "http://myseq:5341/",
    "api_key": "old-key-123"
  },
  "use_partial_hash": true,
  "max_queue_size": 5,
  "mediainfo_path": "MediaInfo\\MediaInfo.exe",
  "ffmpeg_path": "ffmpeg\\bin\\ffmpeg.exe",
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced",
  "file_extensions": [".MOV", ".AVI", ".MKV", ".MP4"],
  "log_level": "DEBUG"
}`
	if err := os.WriteFile(cfgPath, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	cfg, err := cfgpkg.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig with old-format JSON must succeed: %v", err)
	}

	if cfg.Seq.ServerURL != "http://myseq:5341/" {
		t.Errorf("Seq.ServerURL = %q, want %q", cfg.Seq.ServerURL, "http://myseq:5341/")
	}
	if cfg.Seq.APIKey != "old-key-123" {
		t.Errorf("Seq.APIKey = %q, want %q", cfg.Seq.APIKey, "old-key-123")
	}
	if !cfg.UsePartialHash {
		t.Error("UsePartialHash should be true")
	}
	if cfg.MaxQueueSize != 5 {
		t.Errorf("MaxQueueSize = %d, want 5", cfg.MaxQueueSize)
	}
	if cfg.MediaInfoPath != "MediaInfo\\MediaInfo.exe" {
		t.Errorf("MediaInfoPath = %q, want %q", cfg.MediaInfoPath, "MediaInfo\\MediaInfo.exe")
	}
	if cfg.FFmpegPath != "ffmpeg\\bin\\ffmpeg.exe" {
		t.Errorf("FFmpegPath = %q, want %q", cfg.FFmpegPath, "ffmpeg\\bin\\ffmpeg.exe")
	}
	if cfg.VideoEncoder != "hevc_nvenc" {
		t.Errorf("VideoEncoder = %q, want %q", cfg.VideoEncoder, "hevc_nvenc")
	}
	if cfg.QualityPreset != "balanced" {
		t.Errorf("QualityPreset = %q, want %q", cfg.QualityPreset, "balanced")
	}
	// Migration appends missing canonical extensions; verify the original entries
	// from the old config are still present.
	for _, ext := range []string{".MOV", ".AVI", ".MKV", ".MP4"} {
		if !containsExtCompat(cfg.FileExtensions, ext) {
			t.Errorf("FileExtensions missing %q, got %v", ext, cfg.FileExtensions)
		}
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "DEBUG")
	}

	if cfg.MaxEncodesPerGPU != 0 {
		t.Errorf("MaxEncodesPerGPU = %d, want 0", cfg.MaxEncodesPerGPU)
	}
	if cfg.NonInteractive {
		t.Error("NonInteractive should be false")
	}
	if cfg.GPUPreset != "" {
		t.Errorf("GPUPreset = %q, want empty", cfg.GPUPreset)
	}
	if cfg.Rebenchmark {
		t.Error("Rebenchmark should be false")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Various encoder values load without validation error
// ---------------------------------------------------------------------------

func TestBackwardCompatAutoEncoder(t *testing.T) {
	cases := []struct {
		name    string
		encoder string
	}{
		{"auto", "auto"},
		{"hevc_nvenc", "hevc_nvenc"},
		{"libx265", "libx265"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "cfg.json")

			cfg := types.Config{
				VideoEncoder:   tc.encoder,
				FileExtensions: []string{".MP4"},
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}

			loaded, err := cfgpkg.LoadConfig(cfgPath)
			if err != nil {
				t.Fatalf("LoadConfig with encoder=%q must succeed: %v", tc.encoder, err)
			}
			if loaded.VideoEncoder != tc.encoder {
				t.Errorf("VideoEncoder = %q, want %q", loaded.VideoEncoder, tc.encoder)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// moveFile tests
// ---------------------------------------------------------------------------

func TestBackwardCompatDatabase(t *testing.T) {
	dir := t.TempDir()

	dbJSON := `{
  "hashkey1": {
    "original_size": 1000,
    "converted_size": 500,
    "output": "D:\\HSORTED\\test\\file.mp4"
  },
  "hashkey2": {
    "original_size": 2000,
    "note": "not_beneficial"
  },
  "hashkey3": {
    "original_size": 3000,
    "error": "rc_1"
  }
}`
	dbPath := filepath.Join(dir, "converted_files.json")
	if err := os.WriteFile(dbPath, []byte(dbJSON), 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	dbMgr := database.NewDatabaseManager(logger)

	driveRoot := dir + string(filepath.Separator)

	rec1 := dbMgr.GetRecord(driveRoot, "hashkey1")
	if rec1 == nil {
		t.Fatal("GetRecord(hashkey1) returned nil")
	}
	if rec1.OriginalSize != 1000 {
		t.Errorf("OriginalSize = %d, want 1000", rec1.OriginalSize)
	}
	if rec1.ConvertedSize != 500 {
		t.Errorf("ConvertedSize = %d, want 500", rec1.ConvertedSize)
	}
	if rec1.Output != `D:\HSORTED\test\file.mp4` {
		t.Errorf("Output = %q, want %q", rec1.Output, `D:\HSORTED\test\file.mp4`)
	}

	rec2 := dbMgr.GetRecord(driveRoot, "hashkey2")
	if rec2 == nil {
		t.Fatal("GetRecord(hashkey2) returned nil")
	}
	if rec2.Note != "not_beneficial" {
		t.Errorf("Note = %q, want %q", rec2.Note, "not_beneficial")
	}

	rec3 := dbMgr.GetRecord(driveRoot, "hashkey3")
	if rec3 == nil {
		t.Fatal("GetRecord(hashkey3) returned nil")
	}
	if rec3.Error != "rc_1" {
		t.Errorf("Error = %q, want %q", rec3.Error, "rc_1")
	}

	recNil := dbMgr.GetRecord(driveRoot, "nonexistent")
	if recNil != nil {
		t.Errorf("GetRecord(nonexistent) should return nil, got %+v", recNil)
	}

	dbMgr.UpdateRecord(driveRoot, "hashkey4", types.Record{
		OriginalSize:  4000,
		ConvertedSize: 2000,
		Output:        `D:\HSORTED\new\video.mp4`,
	})
	rec4 := dbMgr.GetRecord(driveRoot, "hashkey4")
	if rec4 == nil {
		t.Fatal("GetRecord(hashkey4) returned nil after UpdateRecord")
	}
	if rec4.OriginalSize != 4000 || rec4.ConvertedSize != 2000 {
		t.Errorf("UpdateRecord round-trip failed: got %+v", rec4)
	}
}

// ---------------------------------------------------------------------------
// moveFile tests
// ---------------------------------------------------------------------------

func TestMoveFileSameDrive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	dst := filepath.Join(dir, "dst.mp4")

	content := []byte("video data")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := converter.MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile same-dir: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("dst content = %q, want %q", got, content)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src should be removed after move")
	}
}

func TestMoveFileCrossDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "src.mp4")
	dst := filepath.Join(dstDir, "sub", "dst.mp4")

	content := []byte("cross dir video data")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := converter.MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile cross-dir: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("dst content = %q, want %q", got, content)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src should be removed after move")
	}
}

func TestMoveFileMissingSrc(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nonexistent.mp4")
	dst := filepath.Join(dir, "dst.mp4")

	if err := converter.MoveFile(src, dst); err == nil {
		t.Error("MoveFile with missing src should return error")
	}
}

// ---------------------------------------------------------------------------
// GetDriveRoot
// ---------------------------------------------------------------------------

func TestGetDriveRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		tests := []struct {
			input string
			want  string
		}{
			{`C:\Videos\foo.mp4`, `C:\`},
			{`D:\`, `D:\`},
			{`C:\deep\path\file.mkv`, `C:\`},
		}
		for _, tc := range tests {
			got := fileutil.GetDriveRoot(tc.input)
			if got != tc.want {
				t.Errorf("GetDriveRoot(%q) = %q, want %q", tc.input, got, tc.want)
			}
		}
	}
	if got := fileutil.GetDriveRoot(""); got != "/" {
		t.Errorf("GetDriveRoot(\"\") = %q, want \"/\"", got)
	}
	if got := fileutil.GetDriveRoot("relative/path/file.mp4"); got != "/" {
		t.Errorf("GetDriveRoot(relative) = %q, want \"/\"", got)
	}
}

// ---------------------------------------------------------------------------
// GetParentFolderName
// ---------------------------------------------------------------------------

func TestGetParentFolderName(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("path tests use Windows separators")
	}
	tests := []struct {
		filePath  string
		driveRoot string
		want      string
	}{
		{`C:\Videos\movie.mp4`, `C:\`, "Videos"},
		{`C:\movie.mp4`, `C:\`, "ROOT"},
		{`C:\A\B\movie.mp4`, `C:\`, "B"},
		{`D:\Foo\file.mkv`, `D:\`, "Foo"},
	}
	for _, tc := range tests {
		got := fileutil.GetParentFolderName(tc.filePath, tc.driveRoot)
		if got != tc.want {
			t.Errorf("GetParentFolderName(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// GetRelativePath
// ---------------------------------------------------------------------------

func TestGetRelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("path tests use Windows separators")
	}
	tests := []struct {
		filePath  string
		driveRoot string
		want      string
	}{
		{`C:\Videos\foo.mp4`, `C:\`, "Videos"},
		{`C:\foo.mp4`, `C:\`, "ROOT"},
		{`C:\A\B\foo.mp4`, `C:\`, `A\B`},
	}
	for _, tc := range tests {
		got := fileutil.GetRelativePath(tc.filePath, tc.driveRoot)
		if got != tc.want {
			t.Errorf("GetRelativePath(%q, %q) = %q, want %q", tc.filePath, tc.driveRoot, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SanitizeFolderName
// ---------------------------------------------------------------------------

func TestSanitizeFolderName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Normal", "Normal"},
		{"My:Video", "My_Video"},
		{"A*B?C", "A_B_C"},
		{`foo"bar`, "foo_bar"},
		{"a<b>c", "a_b_c"},
		{"pipe|pipe", "pipe_pipe"},
		{"clean", "clean"},
	}
	for _, tc := range tests {
		got := fileutil.SanitizeFolderName(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeFolderName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatBytes
// ---------------------------------------------------------------------------

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tc := range tests {
		got := fileutil.FormatBytes(tc.bytes)
		if got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FmtElapsed
// ---------------------------------------------------------------------------

func TestFmtElapsed(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "0m 45s"},
		{90 * time.Second, "1m 30s"},
		{60 * time.Second, "1m 00s"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1h 02m 03s"},
		{2*time.Hour + 0*time.Minute + 0*time.Second, "2h 00m 00s"},
	}
	for _, tc := range tests {
		got := fileutil.FmtElapsed(tc.d)
		if got != tc.want {
			t.Errorf("FmtElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TruncateString
// ---------------------------------------------------------------------------

func TestTruncateString(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"hello", 2, "he"},
		{"hello", 1, "h"},
		{"", 5, ""},
	}
	for _, tc := range tests {
		got := fileutil.TruncateString(tc.s, tc.maxLen)
		if got != tc.want {
			t.Errorf("TruncateString(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// BuildConversionArgs
// ---------------------------------------------------------------------------

func TestBuildConversionArgsLibx265MP4(t *testing.T) {
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28", "-preset", "medium"}, nil, nil,
	)
	assertContainsStr(t, args, "-hide_banner")
	assertContainsStr(t, args, "-y")
	assertContainsStr(t, args, "-nostats")
	assertContainsSeqStr(t, args, "-progress", "pipe:1")
	assertContainsSeqStr(t, args, "-i", `C:\input.mp4`)
	assertContainsSeqStr(t, args, "-c:v", "libx265")
	assertContainsSeqStr(t, args, "-crf", "28")
	assertContainsSeqStr(t, args, "-preset", "medium")
	// With nil videoInfo the fallback is -map 0:v:0, -map 0:a?, -c:a copy
	assertContainsSeqStr(t, args, "-map", "0:v:0")
	assertContainsSeqStr(t, args, "-c:a", "copy")
	assertContainsSeqStr(t, args, "-movflags", "+faststart")
	assertContainsSeqStr(t, args, "-map_metadata", "0")
	if args[len(args)-1] != `C:\output.mp4` {
		t.Errorf("last arg should be output path, got %q", args[len(args)-1])
	}
}

func TestBuildConversionArgsWithDeviceArgs(t *testing.T) {
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "hevc_nvenc",
		[]string{"-cq", "28", "-preset", "p5"}, []string{"-gpu", "0"}, nil,
	)
	assertContainsSeqStr(t, args, "-gpu", "0")
	assertContainsSeqStr(t, args, "-c:v", "hevc_nvenc")
	assertContainsSeqStr(t, args, "-cq", "28")
	assertContainsSeqStr(t, args, "-movflags", "+faststart")

	gpuIdx := indexStr(args, "-gpu")
	cvIdx := indexStr(args, "-c:v")
	if gpuIdx == -1 || cvIdx == -1 || gpuIdx >= cvIdx {
		t.Errorf("device args (-gpu) should appear before -c:v; indices: -gpu=%d, -c:v=%d", gpuIdx, cvIdx)
	}
}

func TestBuildConversionArgsMKV(t *testing.T) {
	args := converter.BuildConversionArgs(
		`C:\input.mkv`, `C:\output.mkv`, ".mkv", "libx265",
		[]string{"-crf", "28", "-preset", "medium"}, nil, nil,
	)
	assertContainsSeqStr(t, args, "-c:a", "copy")
	assertContainsSeqStr(t, args, "-c:s", "copy")
	assertContainsStr(t, args, "-map")
	assertNotContainsStr(t, args, "-movflags")
	assertNotContainsStr(t, args, "aac")
	if args[len(args)-1] != `C:\output.mkv` {
		t.Errorf("last arg should be output path, got %q", args[len(args)-1])
	}
}

func TestBuildConversionArgsMP4WithAACAudio(t *testing.T) {
	vi := &types.VideoInfo{
		AudioStreams: []types.AudioStream{
			{CodecName: "aac", Channels: 2},
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	// AAC is MP4-compatible: should be copied, not transcoded.
	assertContainsSeqStr(t, args, "-c:a", "copy")
	assertContainsSeqStr(t, args, "-map", "0:a:0")
	assertContainsSeqStr(t, args, "-movflags", "+faststart")
}

func TestBuildConversionArgsMP4WithDTSAudio(t *testing.T) {
	vi := &types.VideoInfo{
		AudioStreams: []types.AudioStream{
			{CodecName: "dts", Channels: 6},
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	// DTS is not MP4-compatible: must be transcoded to AAC.
	assertContainsStr(t, args, "aac")
	assertContainsSeqStr(t, args, "-movflags", "+faststart")
	// Should NOT contain a bare -c:a copy (only per-stream spec allowed).
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c:a" && args[i+1] == "copy" {
			t.Errorf("DTS audio should not produce -c:a copy\nargs: %s", strings.Join(args, " "))
		}
	}
}

func TestBuildConversionArgsMP4WithMovTextSubtitle(t *testing.T) {
	vi := &types.VideoInfo{
		SubtitleStreams: []types.SubtitleStream{
			{CodecName: "mov_text"},
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	// mov_text is MP4-compatible: subtitle stream should be mapped.
	assertContainsSeqStr(t, args, "-map", "0:s:0")
	assertContainsSeqStr(t, args, "-c:s", "mov_text")
}

func TestBuildConversionArgsMP4DropsPGS(t *testing.T) {
	vi := &types.VideoInfo{
		SubtitleStreams: []types.SubtitleStream{
			{CodecName: "pgssub"},
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	// PGS (image-based) cannot go into MP4 — must be dropped.
	assertNotContainsSeqStr(t, args, "-map", "0:s:0")
}

func TestBuildConversionArgsMP4HDRPassthrough(t *testing.T) {
	vi := &types.VideoInfo{
		Color: types.ColorInfo{
			ColorPrimaries: "bt2020",
			ColorTransfer:  "smpte2084",
			ColorSpace:     "bt2020nc",
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	assertContainsSeqStr(t, args, "-color_primaries", "bt2020")
	assertContainsSeqStr(t, args, "-color_trc", "smpte2084")
	assertContainsSeqStr(t, args, "-colorspace", "bt2020nc")
}

func TestBuildConversionArgsMP4NoHDRForSDR(t *testing.T) {
	vi := &types.VideoInfo{
		Color: types.ColorInfo{
			ColorPrimaries: "bt709",
			ColorTransfer:  "bt709",
			ColorSpace:     "bt709",
		},
	}
	args := converter.BuildConversionArgs(
		`C:\input.mp4`, `C:\output.mp4`, ".mp4", "libx265",
		[]string{"-crf", "28"}, nil, vi,
	)
	// SDR content should not emit HDR colour flags.
	assertNotContainsStr(t, args, "-color_primaries")
	assertNotContainsStr(t, args, "-color_trc")
	assertNotContainsStr(t, args, "-colorspace")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractArgValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func removeArgPair(args []string, flag string) []string {
	var result []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == flag {
			skip = true
			continue
		}
		result = append(result, a)
	}
	return result
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func assertContainsStr(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("args missing %q\nargs: %s", value, strings.Join(args, " "))
}

func assertNotContainsStr(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			t.Errorf("args should NOT contain %q\nargs: %s", value, strings.Join(args, " "))
			return
		}
	}
}

func assertContainsSeqStr(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return
		}
	}
	t.Errorf("args missing sequence %q %q\nargs: %s", key, value, strings.Join(args, " "))
}

func assertNotContainsSeqStr(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			t.Errorf("args should NOT contain sequence %q %q\nargs: %s", key, value, strings.Join(args, " "))
			return
		}
	}
}

func indexStr(args []string, value string) int {
	for i, a := range args {
		if a == value {
			return i
		}
	}
	return -1
}
