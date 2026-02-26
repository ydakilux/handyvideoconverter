package pipeline

import (
	"fmt"
	"testing"

	"video-converter/internal/types"
)

func makeJobs(n int) []types.Job {
	jobs := make([]types.Job, n)
	for i := 0; i < n; i++ {
		jobs[i] = types.Job{
			FilePath:   fmt.Sprintf("video_%d.mp4", i),
			FileHash:   fmt.Sprintf("hash_%d", i),
			FileNumber: i + 1,
			TotalFiles: n,
		}
	}
	return jobs
}

func countByGPU(jobs []types.Job) map[int]int {
	counts := make(map[int]int)
	for _, j := range jobs {
		counts[j.GPUIndex]++
	}
	return counts
}

func TestDistributor(t *testing.T) {
	tests := []struct {
		name       string
		numJobs    int
		benchmarks map[int]float64
		wantCounts map[int]int // expected job count per GPU (±1 for rounding)
		tolerance  int         // allowed rounding deviation per GPU
	}{
		{
			name:    "Proportional_2GPUs_2to1_ratio",
			numJobs: 90,
			benchmarks: map[int]float64{
				0: 120.0,
				1: 60.0,
			},
			wantCounts: map[int]int{0: 60, 1: 30},
			tolerance:  1,
		},
		{
			name:    "SingleGPU",
			numJobs: 10,
			benchmarks: map[int]float64{
				0: 100.0,
			},
			wantCounts: map[int]int{0: 10},
			tolerance:  0,
		},
		{
			name:       "NilBenchmarks_FallbackToGPU0",
			numJobs:    6,
			benchmarks: nil,
			wantCounts: map[int]int{0: 6},
			tolerance:  0,
		},
		{
			name:    "ZeroSpeed_GPU_gets_no_jobs",
			numJobs: 12,
			benchmarks: map[int]float64{
				0: 100.0,
				1: 0.0,
			},
			wantCounts: map[int]int{0: 12, 1: 0},
			tolerance:  0,
		},
		{
			name:       "EmptyBenchmarks_FallbackToGPU0",
			numJobs:    5,
			benchmarks: map[int]float64{},
			wantCounts: map[int]int{0: 5},
			tolerance:  0,
		},
		{
			name:    "Proportional_3GPUs",
			numJobs: 100,
			benchmarks: map[int]float64{
				0: 100.0,
				1: 50.0,
				2: 50.0,
			},
			wantCounts: map[int]int{0: 50, 1: 25, 2: 25},
			tolerance:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDistributor()
			jobs := makeJobs(tt.numJobs)

			result := d.Assign(jobs, tt.benchmarks)

			// Verify total count preserved
			if len(result) != tt.numJobs {
				t.Fatalf("expected %d jobs, got %d", tt.numJobs, len(result))
			}

			// Verify GPU distribution
			counts := countByGPU(result)
			for gpu, want := range tt.wantCounts {
				got := counts[gpu]
				diff := got - want
				if diff < 0 {
					diff = -diff
				}
				if diff > tt.tolerance {
					t.Errorf("GPU %d: want %d±%d jobs, got %d", gpu, want, tt.tolerance, got)
				}
			}

			// Verify no jobs assigned to unexpected GPUs
			for gpu, count := range counts {
				if _, expected := tt.wantCounts[gpu]; !expected && count > 0 {
					t.Errorf("unexpected GPU %d got %d jobs", gpu, count)
				}
			}

			// Verify original job data preserved (FilePath, FileHash)
			for i, j := range result {
				if j.FilePath != jobs[i].FilePath {
					t.Errorf("job %d: FilePath changed from %q to %q", i, jobs[i].FilePath, j.FilePath)
				}
			}
		})
	}
}