package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ydakilux/reforge/internal/gpu/detect"
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

// cpuEncoders lists encoders that should be skipped during GPU benchmarks.
// Used by RunFullBenchmark which is reserved for future multi-encoder comparison.
var cpuEncoders = map[string]bool{
	"libx265": true,
	"libx264": true,
}

// BenchmarkResult holds the result of a single-stream benchmark run.
type BenchmarkResult struct {
	Encoder     string    `json:"encoder"`
	GPUIndex    int       `json:"gpu_index"`
	FPS         float64   `json:"fps"`
	SpeedX      float64   `json:"speed_x"`
	WallClockMs int64     `json:"wall_clock_ms"`
	Timestamp   time.Time `json:"timestamp"`
	CacheKey    string    `json:"cache_key"`
}

// ParallelBenchmarkResult holds results for all parallelism levels tested.
type ParallelBenchmarkResult struct {
	Encoder         string                  `json:"encoder"`
	BestParallelism int                     `json:"best_parallelism"`
	BestFPS         float64                 `json:"best_fps"`
	Runs            map[int]BenchmarkResult `json:"runs"` // key = number of parallel streams
	Timestamp       time.Time               `json:"timestamp"`
	CacheKey        string                  `json:"cache_key"`
}

// BenchmarkCache stores both single-stream and parallel results.
type BenchmarkCache struct {
	Results         map[string]BenchmarkResult         `json:"results"`
	ParallelResults map[string]ParallelBenchmarkResult `json:"parallel_results,omitempty"`
	Version         string                             `json:"version"`
}

// CacheKey returns a deterministic SHA-256-based key for caching a
// single-stream benchmark result. The key encodes the encoder name, GPU
// identifier, and driver version so stale results are invalidated when
// hardware or drivers change.
func CacheKey(encoderName string, gpuIdentifier string, driverVersion string) string {
	raw := fmt.Sprintf("%s|%s|%s", encoderName, gpuIdentifier, driverVersion)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash[:16])
}

// ParallelCacheKey returns the cache key used for a parallel-sweep result.
func ParallelCacheKey(encoderName string) string {
	raw := fmt.Sprintf("parallel|%s", encoderName)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash[:16])
}

// RunBenchmark encodes a synthetic 1080p test source for 20 seconds using the
// specified encoder and returns the measured FPS and wall-clock time. An
// optional deviceArgs slice selects a specific GPU (e.g. ["-gpu", "1"] for
// NVENC multi-GPU setups).
func RunBenchmark(ffmpegPath string, encoder string, gpuIndex int, qualityArgs []string, logger *logrus.Logger, deviceArgs ...[]string) (*BenchmarkResult, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpeg not found — check ffmpeg_path in config or add ffmpeg to PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=duration=%d:size=%dx%d:rate=%d", benchmarkDuration, benchmarkWidth, benchmarkHeight, benchmarkRate),
		"-c:v", encoder,
	}
	if len(deviceArgs) > 0 && len(deviceArgs[0]) > 0 {
		args = append(args, deviceArgs[0]...)
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

// runParallelStreams launches n simultaneous benchmark encodes and returns
// combined FPS = (n * totalFrames) / max(wall clock across all streams).
func runParallelStreams(ffmpegPath, encoder string, n int, qualityArgs []string, logger *logrus.Logger) (*BenchmarkResult, error) {
	type result struct {
		wallMs int64
		err    error
	}

	results := make([]result, n)
	var wg sync.WaitGroup
	wg.Add(n)

	// Use the longest wall-clock as the denominator — that's the real elapsed
	// time the user would wait if running n jobs simultaneously.
	start := time.Now()
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), runTimeout*time.Duration(n))
			defer cancel()

			args := []string{
				"-hide_banner", "-y", "-nostats",
				"-f", "lavfi",
				"-i", fmt.Sprintf("testsrc=duration=%d:size=%dx%d:rate=%d", benchmarkDuration, benchmarkWidth, benchmarkHeight, benchmarkRate),
				"-c:v", encoder,
			}
			args = append(args, qualityArgs...)
			args = append(args, "-f", "null", "-")

			cmd := exec.CommandContext(ctx, ffmpegPath, args...)
			_, err := cmd.CombinedOutput()
			results[i] = result{wallMs: time.Since(start).Milliseconds(), err: err}
		}()
	}
	wg.Wait()

	wallMs := time.Since(start).Milliseconds()
	if wallMs <= 0 {
		wallMs = 1
	}

	for i, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("parallel stream %d/%d failed: %w", i+1, n, r.err)
		}
	}

	combinedFrames := float64(n * totalFrames)
	fps := combinedFrames / (float64(wallMs) / 1000.0)
	speedX := fps / float64(benchmarkRate)

	return &BenchmarkResult{
		Encoder:     encoder,
		FPS:         fps,
		SpeedX:      speedX,
		WallClockMs: wallMs,
		Timestamp:   time.Now(),
	}, nil
}

// RunParallelSweep tests parallelism levels 1..maxParallel for the given encoder,
// picks the best level, logs a comparison table, and returns the full result.
func RunParallelSweep(ffmpegPath, encoder string, maxParallel int, qualityArgs []string, logger *logrus.Logger) (*ParallelBenchmarkResult, error) {
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpeg not found — check ffmpeg_path in config or add ffmpeg to PATH")
	}
	if maxParallel < 1 {
		maxParallel = 1
	}

	result := &ParallelBenchmarkResult{
		Encoder:   encoder,
		Runs:      make(map[int]BenchmarkResult),
		Timestamp: time.Now(),
		CacheKey:  ParallelCacheKey(encoder),
	}

	logger.Infof("┌─────────────────────────────────────────────────────┐")
	logger.Infof("│         PARALLEL PERFORMANCE SWEEP                  │")
	logger.Infof("│  Testing %s with 1..%d parallel jobs          │", encoder, maxParallel)
	logger.Infof("├──────────────┬──────────────┬───────────────────────┤")
	logger.Infof("│  Parallel    │  Total FPS   │  vs 1 stream          │")
	logger.Infof("├──────────────┼──────────────┼───────────────────────┤")

	var baseFPS float64

	for n := 1; n <= maxParallel; n++ {
		logger.Infof("  Running %d parallel stream(s)...", n)

		r, err := runParallelStreams(ffmpegPath, encoder, n, qualityArgs, logger)
		if err != nil {
			logger.Warnf("  Parallel=%d failed: %v — stopping sweep", n, err)
			break
		}

		result.Runs[n] = *r

		if n == 1 {
			baseFPS = r.FPS
		}

		scaling := 0.0
		if baseFPS > 0 {
			scaling = (r.FPS / baseFPS) * 100
		}

		logger.Infof("│  %-12d│  %8.1f fps│  %+.1f%%                 │", n, r.FPS, scaling-100)

		if r.FPS > result.BestFPS {
			result.BestFPS = r.FPS
			result.BestParallelism = n
		}
	}

	logger.Infof("└──────────────┴──────────────┴───────────────────────┘")
	logger.Infof("► Best setting: %d parallel job(s) @ %.1f FPS", result.BestParallelism, result.BestFPS)

	return result, nil
}

// RunFullBenchmark benchmarks all available GPU encoders with warm-up runs.
// Reserved for future multi-encoder comparison; currently only exercised by tests.
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

// LoadCache reads the benchmark cache JSON file from the directory containing
// configPath. Returns an empty cache (not nil) if the file does not exist yet.
func LoadCache(configPath string) (*BenchmarkCache, error) {
	emptyCache := &BenchmarkCache{
		Results:         make(map[string]BenchmarkResult),
		ParallelResults: make(map[string]ParallelBenchmarkResult),
		Version:         "1",
	}

	cachePath := CachePath(configPath)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyCache, nil
		}
		return nil, fmt.Errorf("read benchmark cache: %w", err)
	}

	var cache BenchmarkCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse benchmark cache: %w", err)
	}
	if cache.Results == nil {
		cache.Results = make(map[string]BenchmarkResult)
	}
	if cache.ParallelResults == nil {
		cache.ParallelResults = make(map[string]ParallelBenchmarkResult)
	}

	return &cache, nil
}

// SaveCache writes the benchmark cache to a JSON file beside configPath,
// using an atomic write-tmp-then-rename strategy.
func SaveCache(configPath string, cache *BenchmarkCache) error {
	cachePath := CachePath(configPath)

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal benchmark cache: %w", err)
	}

	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write benchmark cache tmp: %w", err)
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return fmt.Errorf("rename benchmark cache: %w", err)
	}

	return nil
}

// CachePath returns the path to the benchmark cache file that lives
// beside the config file. The config file itself is never modified.
func CachePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "benchmark_cache.json")
}

// IsCacheValid reports whether a single-stream benchmark result for key exists
// in cache and is younger than 30 days.
func IsCacheValid(cache *BenchmarkCache, key string) bool {
	result, ok := cache.Results[key]
	if !ok {
		return false
	}
	return time.Since(result.Timestamp) < cacheMaxAge
}

// IsParallelCacheValid returns true if a valid parallel sweep result exists for the given key.
func IsParallelCacheValid(cache *BenchmarkCache, key string) bool {
	result, ok := cache.ParallelResults[key]
	if !ok {
		return false
	}
	if result.BestFPS <= 0 {
		return false
	}
	return time.Since(result.Timestamp) < cacheMaxAge
}
