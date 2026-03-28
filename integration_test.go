package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	cfgpkg "video-converter/internal/config"
	"video-converter/internal/encoder"
	"video-converter/internal/fallback"
	"video-converter/internal/gpu/benchmark"
	"video-converter/internal/gpu/detect"
)

// mockGPUEncoder implements encoder.Encoder for testing FallbackManager.
// It always reports GPU errors from ParseError.
type mockGPUEncoder struct {
	name string
}

func (m *mockGPUEncoder) Name() string { return m.name }

func (m *mockGPUEncoder) QualityArgs(preset string, width int) []string {
	return []string{"-cq", "28", "-preset", "p5"}
}

func (m *mockGPUEncoder) DeviceArgs(gpuIndex int) []string {
	return []string{"-gpu", "0"}
}

func (m *mockGPUEncoder) IsAvailable(ffmpegPath string) bool { return false }

func (m *mockGPUEncoder) ParseError(stderr string) (bool, string) {
	return true, "mock GPU error: device unavailable"
}

// mockJobStringer implements fmt.Stringer for FallbackManager.HandleGPUError.
type mockJobStringer struct {
	name string
}

func (j *mockJobStringer) String() string { return j.name }

// testLogger creates a quiet logger for tests.
func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.WarnLevel)
	return l
}

// --- Test 1: Auto encoder selection with no GPU available ---

func TestIntegrationAutoEncoderFallback(t *testing.T) {
	// When detection runs with a non-existent ffmpeg path, it should
	// still return a DetectionResult where no GPU encoder is available,
	// and SelectBest should fall back to libx265 (the CPU fallback).

	logger := testLogger()

	// Use a bogus ffmpeg path — detection will fail to list/trial encode
	// any GPU encoders, but DetectEncoders returns an error-free result
	// with CPUFallback always available.
	result, err := detect.DetectEncoders("nonexistent_ffmpeg_binary_for_test", logger)
	if err != nil {
		t.Fatalf("DetectEncoders returned unexpected error: %v", err)
	}

	// No GPU encoder should be available
	for _, gpu := range result.Available {
		if gpu.Encoder != "libx265" && gpu.Available {
			t.Errorf("GPU encoder %s should NOT be available without ffmpeg, but was", gpu.Encoder)
		}
	}

	// CPU fallback must be available
	if !result.CPUFallback.Available {
		t.Fatal("CPUFallback should always be available")
	}
	if result.CPUFallback.Encoder != "libx265" {
		t.Errorf("CPUFallback encoder = %q, want %q", result.CPUFallback.Encoder, "libx265")
	}

	// SelectBest should return libx265
	best := detect.SelectBest(result)
	if best.Encoder != "libx265" {
		t.Errorf("SelectBest = %q, want %q", best.Encoder, "libx265")
	}

	// Verify the full flow: registry lookup with the selected encoder
	registry := encoder.NewRegistry()
	registry.Register(encoder.NewLibx265Encoder())
	registry.Register(encoder.NewNvencEncoder())
	registry.Register(encoder.NewAmfEncoder())
	registry.Register(encoder.NewQsvEncoder())

	enc, ok := registry.Get(best.Encoder)
	if !ok {
		t.Fatalf("Registry.Get(%q) returned false", best.Encoder)
	}
	if enc.Name() != "libx265" {
		t.Errorf("selected encoder name = %q, want %q", enc.Name(), "libx265")
	}

	t.Logf("Auto-detection correctly fell back to %s", enc.Name())
}

// --- Test 2: Encoder registry + explicit selection ---

func TestIntegrationEncoderRegistryExplicitSelection(t *testing.T) {
	registry := encoder.NewRegistry()
	registry.Register(encoder.NewLibx265Encoder())
	registry.Register(encoder.NewNvencEncoder())
	registry.Register(encoder.NewAmfEncoder())
	registry.Register(encoder.NewQsvEncoder())

	// Sub-test: explicit libx265 request returns correct encoder
	t.Run("ExplicitLibx265", func(t *testing.T) {
		enc, ok := registry.Get("libx265")
		if !ok {
			t.Fatal("Registry.Get('libx265') = false")
		}
		if enc.Name() != "libx265" {
			t.Errorf("Name() = %q, want %q", enc.Name(), "libx265")
		}

		// Verify quality args are produced
		args := enc.QualityArgs("balanced", 1920)
		if len(args) == 0 {
			t.Error("QualityArgs returned empty slice")
		}
		// libx265 uses -crf
		found := false
		for _, a := range args {
			if a == "-crf" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("libx265 QualityArgs missing -crf flag: %v", args)
		}
	})

	// Sub-test: explicit nvenc request
	t.Run("ExplicitNvenc", func(t *testing.T) {
		enc, ok := registry.Get("hevc_nvenc")
		if !ok {
			t.Fatal("Registry.Get('hevc_nvenc') = false")
		}
		if enc.Name() != "hevc_nvenc" {
			t.Errorf("Name() = %q, want %q", enc.Name(), "hevc_nvenc")
		}

		// NVENC uses -cq instead of -crf
		args := enc.QualityArgs("balanced", 1920)
		found := false
		for _, a := range args {
			if a == "-cq" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("hevc_nvenc QualityArgs missing -cq flag: %v", args)
		}
	})

	// Sub-test: unknown encoder falls back to libx265 (mimics main.go logic)
	t.Run("UnknownEncoderFallback", func(t *testing.T) {
		_, ok := registry.Get("hevc_imaginary")
		if ok {
			t.Fatal("Registry.Get('hevc_imaginary') should return false")
		}

		// Mimic main.go fallback: when Get fails, use libx265
		fallbackEnc, ok := registry.Get("libx265")
		if !ok {
			t.Fatal("libx265 must always be available as fallback")
		}
		if fallbackEnc.Name() != "libx265" {
			t.Errorf("fallback encoder name = %q, want %q", fallbackEnc.Name(), "libx265")
		}
	})

	// Sub-test: All() returns all 4 encoders in registration order
	t.Run("AllEncoders", func(t *testing.T) {
		all := registry.All()
		if len(all) != 4 {
			t.Fatalf("All() returned %d encoders, want 4", len(all))
		}
		expected := []string{"libx265", "hevc_nvenc", "hevc_amf", "hevc_qsv"}
		for i, enc := range all {
			if enc.Name() != expected[i] {
				t.Errorf("All()[%d].Name() = %q, want %q", i, enc.Name(), expected[i])
			}
		}
	})
}

// --- Test 3: Config backward compatibility ---

func TestIntegrationConfigBackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a minimal "old-style" config JSON with NO GPU-related fields
	oldConfig := map[string]interface{}{
		"seq": map[string]interface{}{
			"enabled":    false,
			"server_url": "http://localhost:5341/",
			"api_key":    "",
		},
		"use_partial_hash": true,
		"max_queue_size":   3,
		"mediainfo_path":   "MediaInfo.exe",
		"ffmpeg_path":      "ffmpeg.exe",
		"ffprobe_path":     "",
		"temp_directory":   "",
		"video_encoder":    "libx265",
		"quality_preset":   "balanced",
		"file_extensions":  []string{".MP4", ".MKV"},
		"log_level":        "INFO",
	}

	configPath := filepath.Join(tmpDir, "config.json")
	data, err := json.MarshalIndent(oldConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal old config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Load it — should NOT error
	cfg, err := cfgpkg.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed on old-style config: %v", err)
	}

	// GPU fields should be zero-valued
	if cfg.MaxEncodesPerGPU != 0 {
		t.Errorf("MaxEncodesPerGPU = %d before ApplyGPUDefaults, want 0", cfg.MaxEncodesPerGPU)
	}
	if cfg.NonInteractive {
		t.Error("NonInteractive should be false for old config")
	}

	// Apply GPU defaults
	cfgpkg.ApplyGPUDefaults(&cfg)

	if cfg.MaxEncodesPerGPU != 2 {
		t.Errorf("MaxEncodesPerGPU = %d after ApplyGPUDefaults, want 2", cfg.MaxEncodesPerGPU)
	}

	// Verify non-GPU fields survived
	if cfg.VideoEncoder != "libx265" {
		t.Errorf("VideoEncoder = %q, want %q", cfg.VideoEncoder, "libx265")
	}
	if cfg.QualityPreset != "balanced" {
		t.Errorf("QualityPreset = %q, want %q", cfg.QualityPreset, "balanced")
	}
	if cfg.MaxQueueSize != 3 {
		t.Errorf("MaxQueueSize = %d, want 3", cfg.MaxQueueSize)
	}

	t.Log("Old-style config loaded and GPU defaults applied successfully")
}

// --- Test 4: FallbackManager non-interactive mode ---

func TestIntegrationFallbackManagerNonInteractive(t *testing.T) {
	logger := testLogger()

	// Create FallbackManager with interactive=false
	fm := fallback.NewFallbackManager(false, strings.NewReader(""), logger)

	// Create a mock GPU encoder that always reports GPU errors
	mockEnc := &mockGPUEncoder{name: "hevc_nvenc"}

	// Create a mock job stringer
	job := &mockJobStringer{name: "test_video.mp4"}

	// HandleGPUError should return shouldFallback=true without reading stdin
	shouldFallback, err := fm.HandleGPUError("No capable devices found", mockEnc, job)
	if err != nil {
		t.Fatalf("HandleGPUError returned error: %v", err)
	}
	if !shouldFallback {
		t.Error("HandleGPUError should return true for non-interactive mode with GPU error")
	}

	t.Log("FallbackManager correctly auto-fell back in non-interactive mode")
}

// --- Test 5: Benchmark cache round-trip ---

func TestIntegrationBenchmarkCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Start with an empty config file
	emptyJSON := []byte("{}")
	if err := os.WriteFile(configPath, emptyJSON, 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Create a benchmark cache with a test entry
	key := benchmark.CacheKey("hevc_nvenc", "NVIDIA RTX 3080", "531.42")
	now := time.Now()

	cache := &benchmark.BenchmarkCache{
		Results: map[string]benchmark.BenchmarkResult{
			key: {
				Encoder:     "hevc_nvenc",
				GPUIndex:    0,
				FPS:         142.5,
				SpeedX:      4.75,
				WallClockMs: 12300,
				Timestamp:   now,
				CacheKey:    key,
			},
		},
		Version: "1",
	}

	// Save cache to disk
	if err := benchmark.SaveCache(configPath, cache); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	// Load it back
	loaded, err := benchmark.LoadCache(configPath)
	if err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}

	// Verify the entry survived round-trip
	result, ok := loaded.Results[key]
	if !ok {
		t.Fatalf("Loaded cache missing key %q", key)
	}
	if result.Encoder != "hevc_nvenc" {
		t.Errorf("Encoder = %q, want %q", result.Encoder, "hevc_nvenc")
	}
	if result.FPS != 142.5 {
		t.Errorf("FPS = %f, want 142.5", result.FPS)
	}
	if result.SpeedX != 4.75 {
		t.Errorf("SpeedX = %f, want 4.75", result.SpeedX)
	}
	if result.WallClockMs != 12300 {
		t.Errorf("WallClockMs = %d, want 12300", result.WallClockMs)
	}
	if result.GPUIndex != 0 {
		t.Errorf("GPUIndex = %d, want 0", result.GPUIndex)
	}

	// Verify IsCacheValid works
	if !benchmark.IsCacheValid(loaded, key) {
		t.Error("IsCacheValid should return true for freshly-saved entry")
	}

	// Verify IsCacheValid returns false for unknown keys
	if benchmark.IsCacheValid(loaded, "nonexistent_key") {
		t.Error("IsCacheValid should return false for unknown key")
	}

	// Verify the config file was NOT modified (cache is now in a separate file)
	rawData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawData, &raw); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}
	if _, hasBench := raw["benchmark_cache"]; hasBench {
		t.Error("Config file must NOT contain benchmark_cache key after migration to separate file")
	}

	// Verify the cache file exists beside the config
	cachePath := benchmark.CachePath(configPath)
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("benchmark_cache.json not found: %v", err)
	}

	t.Logf("Benchmark cache round-trip succeeded for key %s", key[:16]+"...")
}

// --- Test 6: FallbackManager with non-GPU error (should NOT fallback) ---

func TestIntegrationFallbackManagerNonGPUError(t *testing.T) {
	logger := testLogger()

	fm := fallback.NewFallbackManager(false, strings.NewReader(""), logger)

	// Use a real libx265 encoder — its ParseError always returns (false, "")
	cpuEnc := encoder.NewLibx265Encoder()
	job := &mockJobStringer{name: "test_video.mp4"}

	// For a CPU encoder, HandleGPUError should return false (not a GPU error)
	shouldFallback, err := fm.HandleGPUError("some random ffmpeg error", cpuEnc, job)
	if err != nil {
		t.Fatalf("HandleGPUError returned error: %v", err)
	}
	if shouldFallback {
		t.Error("HandleGPUError should return false for CPU encoder (non-GPU error)")
	}

	t.Log("FallbackManager correctly ignored non-GPU error")
}

// --- Test 8: detect.PriorityOrder and SelectBest with mixed availability ---

func TestIntegrationDetectPriorityOrder(t *testing.T) {
	order := detect.PriorityOrder()
	expected := []string{"hevc_nvenc", "hevc_amf", "hevc_qsv", "libx265"}
	if len(order) != len(expected) {
		t.Fatalf("PriorityOrder length = %d, want %d", len(order), len(expected))
	}
	for i, enc := range order {
		if enc != expected[i] {
			t.Errorf("PriorityOrder[%d] = %q, want %q", i, enc, expected[i])
		}
	}

	// SelectBest with only AMD available should pick hevc_amf
	result := &detect.DetectionResult{
		Available: []detect.GPUInfo{
			{Name: "NVIDIA NVENC", Encoder: "hevc_nvenc", Available: false},
			{Name: "AMD AMF", Encoder: "hevc_amf", Available: true},
			{Name: "Intel QSV", Encoder: "hevc_qsv", Available: false},
			{Name: "CPU x265", Encoder: "libx265", Available: true},
		},
		CPUFallback: detect.GPUInfo{Name: "CPU x265", Encoder: "libx265", Available: true},
	}

	best := detect.SelectBest(result)
	if best.Encoder != "hevc_amf" {
		t.Errorf("SelectBest = %q, want hevc_amf (highest priority available)", best.Encoder)
	}

	// With nothing available except CPU, should get libx265
	resultCPUOnly := &detect.DetectionResult{
		Available: []detect.GPUInfo{
			{Name: "NVIDIA NVENC", Encoder: "hevc_nvenc", Available: false},
			{Name: "AMD AMF", Encoder: "hevc_amf", Available: false},
			{Name: "Intel QSV", Encoder: "hevc_qsv", Available: false},
			{Name: "CPU x265", Encoder: "libx265", Available: true},
		},
		CPUFallback: detect.GPUInfo{Name: "CPU x265", Encoder: "libx265", Available: true},
	}

	bestCPU := detect.SelectBest(resultCPUOnly)
	if bestCPU.Encoder != "libx265" {
		t.Errorf("SelectBest (CPU only) = %q, want libx265", bestCPU.Encoder)
	}

	t.Log("Priority order and SelectBest logic verified")
}

// Ensure unused imports don't cause issues
var _ = fmt.Sprintf
