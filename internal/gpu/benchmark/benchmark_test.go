package benchmark

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"video-converter/internal/gpu/detect"
)

func TestCacheKeyDeterministic(t *testing.T) {
	key1 := CacheKey("hevc_nvenc", "NVIDIA GeForce RTX 3080", "546.33")
	key2 := CacheKey("hevc_nvenc", "NVIDIA GeForce RTX 3080", "546.33")
	if key1 != key2 {
		t.Errorf("CacheKey not deterministic: %q != %q", key1, key2)
	}
	if key1 == "" {
		t.Error("CacheKey returned empty string")
	}
}

func TestCacheKeyChangesWithDriver(t *testing.T) {
	key1 := CacheKey("hevc_nvenc", "NVIDIA GeForce RTX 3080", "546.33")
	key2 := CacheKey("hevc_nvenc", "NVIDIA GeForce RTX 3080", "550.00")
	if key1 == key2 {
		t.Errorf("CacheKey should change when driver version changes: both returned %q", key1)
	}
}

func TestCacheKeyChangesWithEncoder(t *testing.T) {
	key1 := CacheKey("hevc_nvenc", "NVIDIA GeForce RTX 3080", "546.33")
	key2 := CacheKey("hevc_amf", "NVIDIA GeForce RTX 3080", "546.33")
	if key1 == key2 {
		t.Errorf("CacheKey should change when encoder changes: both returned %q", key1)
	}
}

func TestLoadSaveCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	existingConfig := map[string]interface{}{
		"video_encoder":  "hevc_nvenc",
		"quality_preset": "balanced",
	}
	data, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal existing config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write existing config: %v", err)
	}

	cache := &BenchmarkCache{
		Version: "1",
		Results: map[string]BenchmarkResult{
			"test-key-1": {
				Encoder:     "hevc_nvenc",
				GPUIndex:    0,
				FPS:         245.5,
				SpeedX:      8.18,
				WallClockMs: 2446,
				Timestamp:   time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
				CacheKey:    "test-key-1",
			},
		},
	}

	if err := SaveCache(configPath, cache); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	loaded, err := LoadCache(configPath)
	if err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}

	if loaded.Version != cache.Version {
		t.Errorf("Version mismatch: got %q, want %q", loaded.Version, cache.Version)
	}
	if len(loaded.Results) != len(cache.Results) {
		t.Fatalf("Results count mismatch: got %d, want %d", len(loaded.Results), len(cache.Results))
	}

	result, ok := loaded.Results["test-key-1"]
	if !ok {
		t.Fatal("Missing result for key 'test-key-1'")
	}
	if result.Encoder != "hevc_nvenc" {
		t.Errorf("Encoder mismatch: got %q", result.Encoder)
	}
	if result.GPUIndex != 0 {
		t.Errorf("GPUIndex mismatch: got %d", result.GPUIndex)
	}
	if result.FPS != 245.5 {
		t.Errorf("FPS mismatch: got %f", result.FPS)
	}
	if result.SpeedX != 8.18 {
		t.Errorf("SpeedX mismatch: got %f", result.SpeedX)
	}
	if result.WallClockMs != 2446 {
		t.Errorf("WallClockMs mismatch: got %d", result.WallClockMs)
	}
	if !result.Timestamp.Equal(cache.Results["test-key-1"].Timestamp) {
		t.Errorf("Timestamp mismatch: got %v", result.Timestamp)
	}
	if result.CacheKey != "test-key-1" {
		t.Errorf("CacheKey mismatch: got %q", result.CacheKey)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	var fullConfig map[string]interface{}
	if err := json.Unmarshal(raw, &fullConfig); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}
	if fullConfig["video_encoder"] != "hevc_nvenc" {
		t.Errorf("Existing config field 'video_encoder' not preserved: got %v", fullConfig["video_encoder"])
	}
	if fullConfig["quality_preset"] != "balanced" {
		t.Errorf("Existing config field 'quality_preset' not preserved: got %v", fullConfig["quality_preset"])
	}
}

func TestLoadCacheNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.json")

	cache, err := LoadCache(configPath)
	if err != nil {
		t.Fatalf("LoadCache should not error for non-existent file: %v", err)
	}
	if cache == nil {
		t.Fatal("LoadCache should return empty cache, not nil")
	}
	if len(cache.Results) != 0 {
		t.Errorf("Expected empty results, got %d", len(cache.Results))
	}
}

func TestLoadCacheNoField(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"video_encoder": "libx265"}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cache, err := LoadCache(configPath)
	if err != nil {
		t.Fatalf("LoadCache should not error for missing field: %v", err)
	}
	if cache == nil {
		t.Fatal("LoadCache should return empty cache, not nil")
	}
	if len(cache.Results) != 0 {
		t.Errorf("Expected empty results, got %d", len(cache.Results))
	}
}

func TestRunBenchmarkIntegration(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH, skipping integration test")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	result, err := RunBenchmark(ffmpegPath, "libx265", -1, []string{"-crf", "28", "-preset", "ultrafast"}, logger)
	if err != nil {
		t.Fatalf("RunBenchmark failed: %v", err)
	}

	if result.Encoder != "libx265" {
		t.Errorf("Encoder mismatch: got %q", result.Encoder)
	}
	if result.WallClockMs <= 0 {
		t.Errorf("WallClockMs should be positive: got %d", result.WallClockMs)
	}
	if result.FPS <= 0 {
		t.Errorf("FPS should be positive: got %f", result.FPS)
	}
	if result.SpeedX <= 0 {
		t.Errorf("SpeedX should be positive: got %f", result.SpeedX)
	}
}

func TestParallelCacheKeyDeterministic(t *testing.T) {
	k1 := ParallelCacheKey("hevc_nvenc")
	k2 := ParallelCacheKey("hevc_nvenc")
	if k1 != k2 {
		t.Errorf("ParallelCacheKey not deterministic: %q != %q", k1, k2)
	}
	if k1 == "" {
		t.Error("ParallelCacheKey returned empty string")
	}
}

func TestParallelCacheKeyDiffersFromSingle(t *testing.T) {
	single := CacheKey("hevc_nvenc", "", "")
	parallel := ParallelCacheKey("hevc_nvenc")
	if single == parallel {
		t.Error("ParallelCacheKey should differ from single-stream CacheKey")
	}
}

func TestIsParallelCacheValidMissing(t *testing.T) {
	cache := &BenchmarkCache{
		Results:         make(map[string]BenchmarkResult),
		ParallelResults: make(map[string]ParallelBenchmarkResult),
		Version:         "1",
	}
	if IsParallelCacheValid(cache, "nonexistent") {
		t.Error("IsParallelCacheValid should return false for missing key")
	}
}

func TestIsParallelCacheValidZeroFPS(t *testing.T) {
	cache := &BenchmarkCache{
		Results: make(map[string]BenchmarkResult),
		ParallelResults: map[string]ParallelBenchmarkResult{
			"key1": {
				Encoder:         "hevc_nvenc",
				BestParallelism: 2,
				BestFPS:         0, // zero FPS — should be invalid
				Timestamp:       time.Now(),
				CacheKey:        "key1",
			},
		},
		Version: "1",
	}
	if IsParallelCacheValid(cache, "key1") {
		t.Error("IsParallelCacheValid should return false when BestFPS == 0")
	}
}

func TestIsParallelCacheValidExpired(t *testing.T) {
	cache := &BenchmarkCache{
		Results: make(map[string]BenchmarkResult),
		ParallelResults: map[string]ParallelBenchmarkResult{
			"key1": {
				Encoder:         "hevc_nvenc",
				BestParallelism: 2,
				BestFPS:         120.0,
				Timestamp:       time.Now().Add(-31 * 24 * time.Hour), // 31 days ago
				CacheKey:        "key1",
			},
		},
		Version: "1",
	}
	if IsParallelCacheValid(cache, "key1") {
		t.Error("IsParallelCacheValid should return false for expired entry")
	}
}

func TestIsParallelCacheValidFresh(t *testing.T) {
	cache := &BenchmarkCache{
		Results: make(map[string]BenchmarkResult),
		ParallelResults: map[string]ParallelBenchmarkResult{
			"key1": {
				Encoder:         "hevc_nvenc",
				BestParallelism: 2,
				BestFPS:         120.0,
				Timestamp:       time.Now(),
				CacheKey:        "key1",
			},
		},
		Version: "1",
	}
	if !IsParallelCacheValid(cache, "key1") {
		t.Error("IsParallelCacheValid should return true for fresh valid entry")
	}
}

func TestParallelResultsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	initial := map[string]interface{}{"video_encoder": "hevc_nvenc"}
	data, _ := json.MarshalIndent(initial, "", "  ")
	os.WriteFile(configPath, data, 0644)

	key := ParallelCacheKey("hevc_nvenc")
	cache := &BenchmarkCache{
		Results: make(map[string]BenchmarkResult),
		ParallelResults: map[string]ParallelBenchmarkResult{
			key: {
				Encoder:         "hevc_nvenc",
				BestParallelism: 3,
				BestFPS:         310.5,
				Runs: map[int]BenchmarkResult{
					1: {Encoder: "hevc_nvenc", FPS: 120.0},
					2: {Encoder: "hevc_nvenc", FPS: 230.0},
					3: {Encoder: "hevc_nvenc", FPS: 310.5},
					4: {Encoder: "hevc_nvenc", FPS: 295.0},
				},
				Timestamp: time.Now().UTC().Truncate(time.Second),
				CacheKey:  key,
			},
		},
		Version: "1",
	}

	if err := SaveCache(configPath, cache); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	loaded, err := LoadCache(configPath)
	if err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}

	r, ok := loaded.ParallelResults[key]
	if !ok {
		t.Fatal("parallel result not found after round-trip")
	}
	if r.BestParallelism != 3 {
		t.Errorf("BestParallelism: got %d, want 3", r.BestParallelism)
	}
	if r.BestFPS != 310.5 {
		t.Errorf("BestFPS: got %f, want 310.5", r.BestFPS)
	}
	if len(r.Runs) != 4 {
		t.Errorf("Runs count: got %d, want 4", len(r.Runs))
	}
}

func TestRunParallelSweepIntegration(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH, skipping integration test")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Use libx265 ultrafast so the sweep completes quickly in CI
	result, err := RunParallelSweep(ffmpegPath, "libx265", 2, []string{"-crf", "28", "-preset", "ultrafast"}, logger)
	if err != nil {
		t.Fatalf("RunParallelSweep failed: %v", err)
	}

	if result.BestParallelism < 1 || result.BestParallelism > 2 {
		t.Errorf("BestParallelism out of range: got %d", result.BestParallelism)
	}
	if result.BestFPS <= 0 {
		t.Errorf("BestFPS should be positive: got %f", result.BestFPS)
	}
	if len(result.Runs) == 0 {
		t.Error("Runs should not be empty")
	}
}

func TestRunFullBenchmarkSkipsCPU(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH, skipping integration test")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	encoders := []detect.GPUInfo{
		{Name: "CPU x265", Encoder: "libx265", DeviceIndex: -1, Available: true},
	}
	qualityArgs := map[string][]string{
		"libx265": {"-crf", "28", "-preset", "ultrafast"},
	}

	results, err := RunFullBenchmark(ffmpegPath, encoders, qualityArgs, logger)
	if err != nil {
		t.Fatalf("RunFullBenchmark failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for CPU-only encoders, got %d", len(results))
	}
}
