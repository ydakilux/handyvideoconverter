package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/sirupsen/logrus"

	"video-converter/internal/gpu/detect"
)

const (
	benchmarkDuration = 20
	benchmarkWidth    = 1920
	benchmarkHeight   = 1080
	benchmarkRate     = 30
	totalFrames       = benchmarkDuration * benchmarkRate
	runTimeout        = 90 * time.Second
	cacheMaxAge       = 30 * 24 * time.Hour
	runsPerEncoder    = 3
)

var cpuEncoders = map[string]bool{
	"libx265": true,
	"libx264": true,
}

type BenchmarkResult struct {
	Encoder     string    `json:"encoder"`
	GPUIndex    int       `json:"gpu_index"`
	FPS         float64   `json:"fps"`
	SpeedX      float64   `json:"speed_x"`
	WallClockMs int64     `json:"wall_clock_ms"`
	Timestamp   time.Time `json:"timestamp"`
	CacheKey    string    `json:"cache_key"`
}

type BenchmarkCache struct {
	Results map[string]BenchmarkResult `json:"results"`
	Version string                      `json:"version"`
}

func CacheKey(encoderName string, gpuIdentifier string, driverVersion string) string {
	raw := fmt.Sprintf("%s|%s|%s", encoderName, gpuIdentifier, driverVersion)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash[:16])
}

func RunBenchmark(ffmpegPath string, encoder string, gpuIndex int, qualityArgs []string, logger *logrus.Logger) (*BenchmarkResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=duration=%d:size=%dx%d:rate=%d", benchmarkDuration, benchmarkWidth, benchmarkHeight, benchmarkRate),
		"-c:v", encoder,
	}
	args = append(args, qualityArgs...)
	args = append(args, "-f", "null", "-")

	logger.Debugf("Benchmark: %s (GPU %d) starting", encoder, gpuIndex)

	start := time.Now()
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	wallClock := time.Since(start)

	if err != nil {
		logger.Debugf("Benchmark failed for %s: %v — %s", encoder, err, string(output))
		return nil, fmt.Errorf("benchmark %s failed: %w", encoder, err)
	}

	wallMs := wallClock.Milliseconds()
	if wallMs <= 0 {
		wallMs = 1
	}
	fps := float64(totalFrames) / (float64(wallMs) / 1000.0)
	speedX := fps / float64(benchmarkRate)

	logger.Debugf("Benchmark: %s completed in %dms (%.1f FPS, %.2fx)", encoder, wallMs, fps, speedX)

	return &BenchmarkResult{
		Encoder:     encoder,
		GPUIndex:    gpuIndex,
		FPS:         fps,
		SpeedX:      speedX,
		WallClockMs: wallMs,
		Timestamp:   time.Now(),
	}, nil
}

func RunFullBenchmark(ffmpegPath string, encoders []detect.GPUInfo, qualityArgs map[string][]string, logger *logrus.Logger) ([]BenchmarkResult, error) {
	var results []BenchmarkResult

	for _, gpu := range encoders {
		if cpuEncoders[gpu.Encoder] {
			logger.Debugf("Skipping CPU encoder %s in benchmark", gpu.Encoder)
			continue
		}
		if !gpu.Available {
			logger.Debugf("Skipping unavailable encoder %s", gpu.Encoder)
			continue
		}

		args := qualityArgs[gpu.Encoder]
		if args == nil {
			logger.Debugf("No quality args for %s, skipping", gpu.Encoder)
			continue
		}

		var validRuns []BenchmarkResult
		for i := 0; i < runsPerEncoder; i++ {
			logger.Infof("Benchmark run %d/%d for %s", i+1, runsPerEncoder, gpu.Encoder)
			result, err := RunBenchmark(ffmpegPath, gpu.Encoder, gpu.DeviceIndex, args, logger)
			if err != nil {
				logger.Errorf("Benchmark run %d failed for %s: %v", i+1, gpu.Encoder, err)
				return nil, fmt.Errorf("benchmark run %d for %s: %w", i+1, gpu.Encoder, err)
			}

			// Discard first run (warm-up)
			if i > 0 {
				validRuns = append(validRuns, *result)
			} else {
				logger.Debugf("Discarding warm-up run for %s (%.1f FPS)", gpu.Encoder, result.FPS)
			}
		}

		if len(validRuns) == 0 {
			continue
		}

		avgFPS := 0.0
		avgWallMs := int64(0)
		for _, r := range validRuns {
			avgFPS += r.FPS
			avgWallMs += r.WallClockMs
		}
		avgFPS /= float64(len(validRuns))
		avgWallMs /= int64(len(validRuns))

		results = append(results, BenchmarkResult{
			Encoder:     gpu.Encoder,
			GPUIndex:    gpu.DeviceIndex,
			FPS:         avgFPS,
			SpeedX:      avgFPS / float64(benchmarkRate),
			WallClockMs: avgWallMs,
			Timestamp:   time.Now(),
		})
	}

	return results, nil
}

func LoadCache(configPath string) (*BenchmarkCache, error) {
	emptyCache := &BenchmarkCache{
		Results: make(map[string]BenchmarkResult),
		Version: "1",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyCache, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cacheData, ok := raw["benchmark_cache"]
	if !ok {
		return emptyCache, nil
	}

	var cache BenchmarkCache
	if err := json.Unmarshal(cacheData, &cache); err != nil {
		return nil, fmt.Errorf("parse benchmark_cache: %w", err)
	}
	if cache.Results == nil {
		cache.Results = make(map[string]BenchmarkResult)
	}

	return &cache, nil
}

func SaveCache(configPath string, cache *BenchmarkCache) error {
	var raw map[string]json.RawMessage

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			raw = make(map[string]json.RawMessage)
		} else {
			return fmt.Errorf("read config: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}

	cacheJSON, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal benchmark_cache: %w", err)
	}
	raw["benchmark_cache"] = cacheJSON

	output, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, output, 0644); err != nil {
		return fmt.Errorf("write tmp config: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}

func IsCacheValid(cache *BenchmarkCache, key string) bool {
	result, ok := cache.Results[key]
	if !ok {
		return false
	}
	return time.Since(result.Timestamp) < cacheMaxAge
}