package pipeline

import (
	"sort"

	"video-converter/internal/types"
)

// Distributor assigns GPU indices to jobs proportional to measured GPU speeds.
type Distributor struct{}

// NewDistributor creates a new Distributor.
func NewDistributor() *Distributor {
	return &Distributor{}
}

// Assign distributes jobs across GPUs proportional to benchmark speeds.
// benchmarks maps gpuIndex → fps. Jobs are returned with GPUIndex set.
// If benchmarks is nil or empty, all jobs fall back to GPUIndex=0.
// GPUs with speed 0 receive no jobs.
func (d *Distributor) Assign(jobs []types.Job, benchmarks map[int]float64) []types.Job {
	if len(jobs) == 0 {
		return jobs
	}

	// Fallback: nil or empty benchmarks → all GPU 0
	if len(benchmarks) == 0 {
		for i := range jobs {
			jobs[i].GPUIndex = 0
		}
		return jobs
	}

	// Compute total speed (excluding zero-speed GPUs)
	var totalSpeed float64
	for _, fps := range benchmarks {
		if fps > 0 {
			totalSpeed += fps
		}
	}

	// If all GPUs have zero speed, fallback to GPU 0
	if totalSpeed == 0 {
		for i := range jobs {
			jobs[i].GPUIndex = 0
		}
		return jobs
	}

	// Sorted GPU indices for deterministic assignment
	gpuIndices := make([]int, 0, len(benchmarks))
	for idx := range benchmarks {
		gpuIndices = append(gpuIndices, idx)
	}
	sort.Ints(gpuIndices)

	// Calculate proportional slot counts using largest-remainder method
	totalJobs := len(jobs)
	slots := make(map[int]int)
	assigned := 0

	type remainder struct {
		gpuIdx int
		frac   float64
	}
	var remainders []remainder

	for _, idx := range gpuIndices {
		fps := benchmarks[idx]
		if fps <= 0 {
			slots[idx] = 0
			continue
		}
		exact := float64(totalJobs) * fps / totalSpeed
		floor := int(exact)
		slots[idx] = floor
		assigned += floor
		remainders = append(remainders, remainder{gpuIdx: idx, frac: exact - float64(floor)})
	}

	// Distribute remaining jobs by largest fractional remainder
	remaining := totalJobs - assigned
	sort.Slice(remainders, func(i, j int) bool {
		if remainders[i].frac != remainders[j].frac {
			return remainders[i].frac > remainders[j].frac
		}
		return remainders[i].gpuIdx < remainders[j].gpuIdx
	})
	for i := 0; i < remaining && i < len(remainders); i++ {
		slots[remainders[i].gpuIdx]++
	}

	// Assign GPU indices to jobs
	jobIdx := 0
	for _, gpuIdx := range gpuIndices {
		count := slots[gpuIdx]
		for c := 0; c < count && jobIdx < totalJobs; c++ {
			jobs[jobIdx].GPUIndex = gpuIdx
			jobIdx++
		}
	}

	return jobs
}