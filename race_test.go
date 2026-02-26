package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"video-converter/internal/database"
	"video-converter/internal/encoder"
	"video-converter/internal/fallback"
	"video-converter/internal/gpu/benchmark"
	"video-converter/internal/types"
)

func newSilentLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.WarnLevel)
	return l
}

type raceEncoder struct {
	name string
}

func (e *raceEncoder) Name() string                         { return e.name }
func (e *raceEncoder) QualityArgs(_ string, _ int) []string { return nil }
func (e *raceEncoder) DeviceArgs(_ int) []string            { return nil }
func (e *raceEncoder) IsAvailable(_ string) bool            { return true }
func (e *raceEncoder) ParseError(_ string) (bool, string)   { return true, "gpu boom" }

var _ encoder.Encoder = (*raceEncoder)(nil)

type raceJob struct{ id string }

func (j *raceJob) String() string { return j.id }

// TestRaceDatabaseConcurrent hammers UpdateRecord/GetRecord from 10 goroutines
// on the same DatabaseManager and driveRoot. Verifies no data race.
func TestRaceDatabaseConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	dm := database.NewDatabaseManager(newSilentLogger())

	const goroutines = 10
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				hash := fmt.Sprintf("hash_%d_%d", idx, j)
				rec := types.Record{
					OriginalSize:  int64(idx*1000 + j),
					ConvertedSize: int64(idx*500 + j),
					Output:        fmt.Sprintf("out_%d_%d.mp4", idx, j),
				}
				dm.UpdateRecord(driveRoot, hash, rec)

				readHash := fmt.Sprintf("hash_%d_%d", (idx+1)%goroutines, j)
				_ = dm.GetRecord(driveRoot, readHash)
			}
		}(i)
	}

	wg.Wait()

	got := dm.GetRecord(driveRoot, "hash_0_0")
	if got == nil {
		t.Fatal("expected hash_0_0 to exist after concurrent writes")
	}

	dm.SaveAll()
}

// TestRaceFallbackConcurrent calls HandleGPUError from 5 goroutines on the
// same FallbackManager in non-interactive mode. Verifies no data race.
func TestRaceFallbackConcurrent(t *testing.T) {
	fm := fallback.NewFallbackManager(false, strings.NewReader(""), newSilentLogger())
	enc := &raceEncoder{name: "hevc_nvenc"}

	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)
	results := make([]bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			job := &raceJob{id: fmt.Sprintf("video_%d.mp4", idx)}
			shouldFallback, err := fm.HandleGPUError("error: OpenEncodeSessionEx failed", enc, job)
			errs[idx] = err
			results[idx] = shouldFallback
		}(i)
	}

	wg.Wait()

	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if !results[i] {
			t.Errorf("goroutine %d: expected shouldFallback=true in non-interactive mode", i)
		}
	}
}

// TestRaceRegistryConcurrent exercises the Registry with one goroutine
// registering encoders while 8 goroutines read via Get and All. Verifies no race.
func TestRaceRegistryConcurrent(t *testing.T) {
	reg := encoder.NewRegistry()

	const readers = 8
	const encodersToRegister = 20

	reg.Register(&raceEncoder{name: "seed_encoder"})

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < encodersToRegister; i++ {
			reg.Register(&raceEncoder{name: fmt.Sprintf("enc_%d", i)})
			time.Sleep(50 * time.Microsecond)
		}
	}()

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = reg.Get("seed_encoder")
				_, _ = reg.Get(fmt.Sprintf("enc_%d", j%encodersToRegister))
				all := reg.All()
				if len(all) == 0 {
					t.Errorf("reader %d: All() returned empty slice", idx)
				}
			}
		}(i)
	}

	wg.Wait()

	all := reg.All()
	if len(all) != encodersToRegister+1 {
		t.Errorf("expected %d encoders, got %d", encodersToRegister+1, len(all))
	}
}

// TestRaceBenchmarkCacheConcurrent exercises LoadCache/SaveCache with per-goroutine
// config files (matching real usage), and verifies concurrent in-memory
// BenchmarkCache manipulation under a mutex is race-free.
func TestRaceBenchmarkCacheConcurrent(t *testing.T) {
	tmpDir := t.TempDir()

	const goroutines = 6
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			configPath := filepath.Join(tmpDir, fmt.Sprintf("config_%d.json", idx))

			seed := map[string]json.RawMessage{
				"video_encoder": json.RawMessage(`"libx265"`),
			}
			data, _ := json.MarshalIndent(seed, "", "  ")
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				t.Errorf("goroutine %d: seed config: %v", idx, err)
				return
			}

			for j := 0; j < 10; j++ {
				cache, err := benchmark.LoadCache(configPath)
				if err != nil {
					t.Errorf("goroutine %d iter %d: LoadCache: %v", idx, j, err)
					return
				}

				key := benchmark.CacheKey(fmt.Sprintf("enc_%d", idx), "gpu0", "550.0")
				cache.Results[key] = benchmark.BenchmarkResult{
					Encoder:     fmt.Sprintf("enc_%d", idx),
					FPS:         float64(60 + idx + j),
					SpeedX:      2.0,
					WallClockMs: int64(1000 + idx),
					Timestamp:   time.Now(),
					CacheKey:    key,
				}

				if err := benchmark.SaveCache(configPath, cache); err != nil {
					t.Errorf("goroutine %d iter %d: SaveCache: %v", idx, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	for i := 0; i < goroutines; i++ {
		configPath := filepath.Join(tmpDir, fmt.Sprintf("config_%d.json", i))
		cache, err := benchmark.LoadCache(configPath)
		if err != nil {
			t.Errorf("final load goroutine %d: %v", i, err)
			continue
		}
		if len(cache.Results) == 0 {
			t.Errorf("goroutine %d config has no benchmark results", i)
		}
	}

	// Shared in-memory cache with external mutex — mirrors production pattern.
	sharedCache := &benchmark.BenchmarkCache{
		Results: make(map[string]benchmark.BenchmarkResult),
		Version: "1",
	}
	var mu sync.Mutex
	var wg2 sync.WaitGroup
	wg2.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg2.Done()
			for j := 0; j < 20; j++ {
				mu.Lock()
				key := benchmark.CacheKey(fmt.Sprintf("enc_%d_%d", idx, j), "gpu0", "550.0")
				sharedCache.Results[key] = benchmark.BenchmarkResult{
					Encoder:   fmt.Sprintf("enc_%d_%d", idx, j),
					FPS:       float64(60 + idx + j),
					Timestamp: time.Now(),
				}
				_ = benchmark.IsCacheValid(sharedCache, key)
				mu.Unlock()
			}
		}(i)
	}
	wg2.Wait()

	if len(sharedCache.Results) != goroutines*20 {
		t.Errorf("expected %d results in shared cache, got %d", goroutines*20, len(sharedCache.Results))
	}
}

// TestRaceStatsConcurrent simulates concurrent Stats updates that happen during
// pipeline execution. Multiple goroutines increment counters through Stats.Mu.
func TestRaceStatsConcurrent(t *testing.T) {
	stats := &types.Stats{
		TouchedDrives: make(map[string]bool),
	}

	const goroutines = 10
	const updatesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				stats.Mu.Lock()
				stats.FilesAnalyzed++
				stats.FilesProcessed++
				stats.OriginalBytes += int64(1000 + idx)
				stats.FinalBytes += int64(500 + idx)
				stats.TouchedDrives[fmt.Sprintf("drive_%d", idx%3)] = true
				if j%2 == 0 {
					stats.FilesImproved++
				} else {
					stats.FilesDiscarded++
				}
				stats.Mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	expected := goroutines * updatesPerGoroutine
	if stats.FilesAnalyzed != expected {
		t.Errorf("FilesAnalyzed = %d, want %d", stats.FilesAnalyzed, expected)
	}
	if stats.FilesProcessed != expected {
		t.Errorf("FilesProcessed = %d, want %d", stats.FilesProcessed, expected)
	}
	improvedPlusDiscarded := stats.FilesImproved + stats.FilesDiscarded
	if improvedPlusDiscarded != expected {
		t.Errorf("FilesImproved+FilesDiscarded = %d, want %d", improvedPlusDiscarded, expected)
	}
}
