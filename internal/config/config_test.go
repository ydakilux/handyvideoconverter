package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"video-converter/internal/types"
)

func TestLoadConfigReadsExistingJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test_config.json")

	want := types.Config{
		ServerURL:          "http://myserver:5341/",
		APIKey:             "test-key",
		UsePartialHash:     false,
		MaxQueueSize:       5,
		MediaInfoPath:      "custom\\MediaInfo.exe",
		FFmpegPath:         "custom\\ffmpeg.exe",
		FFprobePath:        "custom\\ffprobe.exe",
		TempDirectory:      "C:\\Temp",
		VideoEncoder:       "libx265",
		QualityPreset:      "high_quality",
		CustomQualitySD:    18,
		CustomQuality720p:  20,
		CustomQuality1080p: 22,
		CustomQuality4K:    25,
		FileExtensions:     []string{".MP4", ".MKV"},
		LogLevel:           "DEBUG",
	}

	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got.ServerURL != want.ServerURL {
		t.Errorf("ServerURL = %q, want %q", got.ServerURL, want.ServerURL)
	}
	if got.APIKey != want.APIKey {
		t.Errorf("APIKey = %q, want %q", got.APIKey, want.APIKey)
	}
	if got.UsePartialHash != want.UsePartialHash {
		t.Errorf("UsePartialHash = %v, want %v", got.UsePartialHash, want.UsePartialHash)
	}
	if got.MaxQueueSize != want.MaxQueueSize {
		t.Errorf("MaxQueueSize = %d, want %d", got.MaxQueueSize, want.MaxQueueSize)
	}
	if got.VideoEncoder != want.VideoEncoder {
		t.Errorf("VideoEncoder = %q, want %q", got.VideoEncoder, want.VideoEncoder)
	}
	if got.QualityPreset != want.QualityPreset {
		t.Errorf("QualityPreset = %q, want %q", got.QualityPreset, want.QualityPreset)
	}
	if got.CustomQualitySD != want.CustomQualitySD {
		t.Errorf("CustomQualitySD = %d, want %d", got.CustomQualitySD, want.CustomQualitySD)
	}
	if got.CustomQuality4K != want.CustomQuality4K {
		t.Errorf("CustomQuality4K = %d, want %d", got.CustomQuality4K, want.CustomQuality4K)
	}
	if got.LogLevel != want.LogLevel {
		t.Errorf("LogLevel = %q, want %q", got.LogLevel, want.LogLevel)
	}
	if len(got.FileExtensions) != len(want.FileExtensions) {
		t.Errorf("FileExtensions len = %d, want %d", len(got.FileExtensions), len(want.FileExtensions))
	}
}

func TestCreateDefaultConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "new_config.json")

	cfg, err := CreateDefaultConfig(cfgPath)
	if err != nil {
		t.Fatalf("CreateDefaultConfig: %v", err)
	}

	// File should exist
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Verify defaults
	if cfg.ServerURL != "http://localhost:5341/" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "http://localhost:5341/")
	}
	if cfg.VideoEncoder != "hevc_nvenc" {
		t.Errorf("VideoEncoder = %q, want %q", cfg.VideoEncoder, "hevc_nvenc")
	}
	if cfg.QualityPreset != "balanced" {
		t.Errorf("QualityPreset = %q, want %q", cfg.QualityPreset, "balanced")
	}
	if cfg.MaxQueueSize != 3 {
		t.Errorf("MaxQueueSize = %d, want %d", cfg.MaxQueueSize, 3)
	}
	if !cfg.UsePartialHash {
		t.Error("UsePartialHash should be true")
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "INFO")
	}
	if len(cfg.FileExtensions) != 11 {
		t.Errorf("FileExtensions len = %d, want 11", len(cfg.FileExtensions))
	}

	// Verify file is valid JSON that round-trips
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded types.Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.ServerURL != cfg.ServerURL {
		t.Errorf("round-trip ServerURL mismatch")
	}
}

func TestLoadConfigCreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "missing_config.json")

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Should have created the file with defaults
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
	if cfg.VideoEncoder != "hevc_nvenc" {
		t.Errorf("VideoEncoder = %q, want %q", cfg.VideoEncoder, "hevc_nvenc")
	}
}

func TestDetermineQualityBalancedPreset(t *testing.T) {
	cfg := types.Config{QualityPreset: "balanced"}

	tests := []struct {
		width int
		want  string
	}{
		{640, "23"},  // SD
		{1024, "23"}, // SD boundary
		{1280, "25"}, // 720p
		{1920, "27"}, // 1080p
		{3840, "30"}, // 4K
	}

	for _, tt := range tests {
		got := DetermineQuality(tt.width, cfg)
		if got != tt.want {
			t.Errorf("DetermineQuality(%d, balanced) = %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestDetermineQualityAllPresets(t *testing.T) {
	tests := []struct {
		preset string
		width  int
		want   string
	}{
		{"high_quality", 1920, "21"},
		{"balanced", 1920, "27"},
		{"space_saver", 1920, "30"},
		{"unknown_preset", 1920, "27"}, // default falls back to balanced
	}

	for _, tt := range tests {
		cfg := types.Config{QualityPreset: tt.preset}
		got := DetermineQuality(tt.width, cfg)
		if got != tt.want {
			t.Errorf("DetermineQuality(%d, %s) = %q, want %q", tt.width, tt.preset, got, tt.want)
		}
	}
}

func TestDetermineQualityCustomValues(t *testing.T) {
	cfg := types.Config{
		QualityPreset:      "balanced",
		CustomQualitySD:    15,
		CustomQuality720p:  18,
		CustomQuality1080p: 22,
		CustomQuality4K:    28,
	}

	tests := []struct {
		width int
		want  string
	}{
		{640, "15"},  // Custom SD
		{1024, "15"}, // Custom SD boundary
		{1280, "18"}, // Custom 720p
		{1920, "22"}, // Custom 1080p
		{3840, "28"}, // Custom 4K
	}

	for _, tt := range tests {
		got := DetermineQuality(tt.width, cfg)
		if got != tt.want {
			t.Errorf("DetermineQuality(%d, custom) = %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestAutoEncoderAccepted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "auto_config.json")

	cfg := types.Config{
		VideoEncoder:   "auto",
		FileExtensions: []string{".MP4"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgPath, data, 0644)

	_, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig with auto encoder should succeed, got: %v", err)
	}
}

func TestInvalidEncoderRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad_config.json")

	cfg := types.Config{
		VideoEncoder:   "h264_bogus",
		FileExtensions: []string{".MP4"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgPath, data, 0644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("LoadConfig with invalid encoder should fail")
	}
}

func TestValidateEncoderAllValid(t *testing.T) {
	for _, enc := range ValidEncoders {
		if err := ValidateEncoder(enc); err != nil {
			t.Errorf("ValidateEncoder(%q) should succeed, got: %v", enc, err)
		}
	}
	// Empty is also valid
	if err := ValidateEncoder(""); err != nil {
		t.Errorf("ValidateEncoder(\"\") should succeed, got: %v", err)
	}
}

func TestResolveExecutableEmptyPath(t *testing.T) {
	// Empty configPath should attempt LookPath
	result := ResolveExecutable("", "some_nonexistent_exe_12345", "C:\\fakedir")
	// LookPath for a nonexistent exe should return empty
	if result != "" {
		t.Errorf("expected empty string for nonexistent exe, got %q", result)
	}
}

func TestResolveExecutableAbsolutePath(t *testing.T) {
	// Create a temp file to act as the executable
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "ffmpeg.exe")
	os.WriteFile(fakePath, []byte("fake"), 0755)

	// Absolute path should be returned as-is
	result := ResolveExecutable(fakePath, "ffmpeg.exe", "C:\\irrelevant")
	if result != fakePath {
		t.Errorf("expected %q, got %q", fakePath, result)
	}
}

func TestResolveExecutableRelativePath(t *testing.T) {
	// Create a temp "execDir" with a relative file
	dir := t.TempDir()
	subDir := filepath.Join(dir, "bin")
	os.MkdirAll(subDir, 0755)
	fakePath := filepath.Join(subDir, "ffmpeg.exe")
	os.WriteFile(fakePath, []byte("fake"), 0755)

	// Relative path should be joined with execDir
	result := ResolveExecutable(filepath.Join("bin", "ffmpeg.exe"), "ffmpeg.exe", dir)
	if result != fakePath {
		t.Errorf("expected %q, got %q", fakePath, result)
	}
}


func TestConfigBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "old_config.json")

	// JSON without any GPU fields (simulates old config format)
	oldJSON := `{
	  "server_url": "http://localhost:5341/",
	  "api_key": "",
	  "use_partial_hash": true,
	  "max_queue_size": 3,
	  "video_encoder": "hevc_nvenc",
	  "quality_preset": "balanced",
	  "file_extensions": [".MP4", ".MKV"],
	  "log_level": "INFO"
	}`
	if err := os.WriteFile(cfgPath, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig with old format should succeed, got: %v", err)
	}

	// GPU fields should be at zero values
	if cfg.MaxEncodesPerGPU != 0 {
		t.Errorf("MaxEncodesPerGPU = %d, want 0", cfg.MaxEncodesPerGPU)
	}
	if cfg.NonInteractive {
		t.Error("NonInteractive should be false")
	}
	if cfg.GPUPreset != "" {
		t.Errorf("GPUPreset = %q, want empty", cfg.GPUPreset)
	}
	if cfg.BenchmarkCache != nil {
		t.Errorf("BenchmarkCache should be nil, got %v", cfg.BenchmarkCache)
	}
	if cfg.Rebenchmark {
		t.Error("Rebenchmark should be false")
	}
}

func TestApplyGPUDefaults(t *testing.T) {
	// Zero value → should be set to default
	cfg := types.Config{}
	ApplyGPUDefaults(&cfg)
	if cfg.MaxEncodesPerGPU != 2 {
		t.Errorf("MaxEncodesPerGPU = %d, want 2", cfg.MaxEncodesPerGPU)
	}

	// Non-zero value → should be preserved
	cfg2 := types.Config{MaxEncodesPerGPU: 5}
	ApplyGPUDefaults(&cfg2)
	if cfg2.MaxEncodesPerGPU != 5 {
		t.Errorf("MaxEncodesPerGPU = %d, want 5 (preserved)", cfg2.MaxEncodesPerGPU)
	}
}

func TestBenchmarkCacheRoundTrip(t *testing.T) {
	cfg := types.Config{}
	now := time.Now().Truncate(time.Second)
	entry := types.BenchmarkCacheEntry{
		FPS:         120.5,
		Timestamp:   now,
		EncoderName: "hevc_nvenc",
	}

	// Save to in-memory config (no file needed for this test)
	if cfg.BenchmarkCache == nil {
		cfg.BenchmarkCache = make(map[string]types.BenchmarkCacheEntry)
	}
	cfg.BenchmarkCache["gpu0_hevc_nvenc"] = entry

	got, ok := GetBenchmarkCache(&cfg, "gpu0_hevc_nvenc")
	if !ok {
		t.Fatal("GetBenchmarkCache returned false for existing key")
	}
	if got.FPS != entry.FPS {
		t.Errorf("FPS = %f, want %f", got.FPS, entry.FPS)
	}
	if !got.Timestamp.Equal(entry.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, entry.Timestamp)
	}
	if got.EncoderName != entry.EncoderName {
		t.Errorf("EncoderName = %q, want %q", got.EncoderName, entry.EncoderName)
	}
}

func TestRebenchmarkNotPersisted(t *testing.T) {
	cfg := types.Config{
		VideoEncoder: "hevc_nvenc",
		Rebenchmark:  true,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, "rebenchmark") {
		t.Errorf("JSON should not contain rebenchmark, got: %s", jsonStr)
	}
}

func TestSaveBenchmarkCachePersistence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "persist_config.json")

	cfg := types.Config{
		VideoEncoder:   "hevc_nvenc",
		FileExtensions: []string{".MP4"},
	}
	// Write initial config
	initData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, initData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	entry := types.BenchmarkCacheEntry{
		FPS:         95.3,
		Timestamp:   now,
		EncoderName: "hevc_nvenc",
	}

	if err := SaveBenchmarkCache(cfgPath, &cfg, "gpu0", entry); err != nil {
		t.Fatalf("SaveBenchmarkCache: %v", err)
	}

	// Reload from disk
	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if loaded.BenchmarkCache == nil {
		t.Fatal("BenchmarkCache is nil after reload")
	}
	got, ok := loaded.BenchmarkCache["gpu0"]
	if !ok {
		t.Fatal("gpu0 key not found in reloaded BenchmarkCache")
	}
	if got.FPS != entry.FPS {
		t.Errorf("FPS = %f, want %f", got.FPS, entry.FPS)
	}
	if got.EncoderName != entry.EncoderName {
		t.Errorf("EncoderName = %q, want %q", got.EncoderName, entry.EncoderName)
	}
}

func TestGetBenchmarkCacheMissing(t *testing.T) {
	// nil map
	cfg := types.Config{}
	_, ok := GetBenchmarkCache(&cfg, "nonexistent")
	if ok {
		t.Error("GetBenchmarkCache should return false for nil map")
	}

	// initialized map but missing key
	cfg.BenchmarkCache = make(map[string]types.BenchmarkCacheEntry)
	_, ok = GetBenchmarkCache(&cfg, "nonexistent")
	if ok {
		t.Error("GetBenchmarkCache should return false for missing key")
	}
}