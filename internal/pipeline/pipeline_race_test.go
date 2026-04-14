package pipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ydakilux/reforge/internal/types"
)

// TestPipelineRace verifies the pipeline is free of data races under concurrent use.
// Run with: go test -race ./internal/pipeline/
func TestPipelineRace(t *testing.T) {
	ctx := context.Background()
	consumers := 3
	totalJobs := 10
	p := NewPipeline(ctx, consumers, totalJobs)

	handler := func(ctx context.Context, job types.Job) ConversionResult {
		// Small jitter to create scheduling variation
		time.Sleep(time.Duration(job.FileNumber) * time.Millisecond)
		return ConversionResult{
			Job:        job,
			ExitCode:   0,
			OutputPath: "/out/" + job.FilePath,
			OutputSize: job.OriginalSize / 2,
			Duration:   time.Duration(job.FileNumber) * time.Millisecond,
		}
	}

	p.Start(handler)

	// Submit jobs concurrently from multiple goroutines
	var submitWg sync.WaitGroup
	for i := 0; i < totalJobs; i++ {
		submitWg.Add(1)
		go func(n int) {
			defer submitWg.Done()
			job := types.Job{
				FilePath:     fmt.Sprintf("race_video_%d.mp4", n),
				FileHash:     fmt.Sprintf("race_hash_%d", n),
				OriginalSize: int64(1000 * (n + 1)),
				FileNumber:   n + 1,
				TotalFiles:   totalJobs,
			}
			if err := p.Submit(job); err != nil {
				t.Errorf("Submit failed for job %d: %v", n, err)
			}
		}(i)
	}

	// Wait for all submits before closing jobs channel
	submitWg.Wait()

	// Collect results concurrently
	var results []ConversionResult
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		for r := range p.Results() {
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}
		close(done)
	}()

	p.Wait()
	<-done

	if len(results) != totalJobs {
		t.Fatalf("expected %d results, got %d", totalJobs, len(results))
	}

	// Verify no duplicates and all jobs accounted for
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.Job.FileHash] {
			t.Errorf("duplicate result for hash %s", r.Job.FileHash)
		}
		seen[r.Job.FileHash] = true
	}
}
