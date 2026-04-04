package pipeline

import (
	"testing"
)

func TestGPUAssigner(t *testing.T) {
	tests := []struct {
		name       string
		benchmarks map[int]float64
		calls      int
		wantCounts map[int]int
		tolerance  int
	}{
		{
			name:       "NilBenchmarks_ReturnsZero",
			benchmarks: nil,
			calls:      10,
			wantCounts: map[int]int{0: 10},
			tolerance:  0,
		},
		{
			name:       "EmptyBenchmarks_ReturnsZero",
			benchmarks: map[int]float64{},
			calls:      10,
			wantCounts: map[int]int{0: 10},
			tolerance:  0,
		},
		{
			name:       "AllZeroSpeed_ReturnsZero",
			benchmarks: map[int]float64{0: 0.0, 1: 0.0},
			calls:      10,
			wantCounts: map[int]int{0: 10},
			tolerance:  0,
		},
		{
			name:       "SingleGPU",
			benchmarks: map[int]float64{0: 100.0},
			calls:      50,
			wantCounts: map[int]int{0: 50},
			tolerance:  0,
		},
		{
			name:       "SingleGPU_NonZeroIndex",
			benchmarks: map[int]float64{2: 80.0},
			calls:      20,
			wantCounts: map[int]int{2: 20},
			tolerance:  0,
		},
		{
			name:       "TwoGPUs_2to1_Ratio",
			benchmarks: map[int]float64{0: 120.0, 1: 60.0},
			calls:      300,
			wantCounts: map[int]int{0: 200, 1: 100},
			tolerance:  1,
		},
		{
			name:       "TwoGPUs_EqualSpeed",
			benchmarks: map[int]float64{0: 100.0, 1: 100.0},
			calls:      100,
			wantCounts: map[int]int{0: 50, 1: 50},
			tolerance:  1,
		},
		{
			name:       "ThreeGPUs_Proportional",
			benchmarks: map[int]float64{0: 100.0, 1: 50.0, 2: 50.0},
			calls:      200,
			wantCounts: map[int]int{0: 100, 1: 50, 2: 50},
			tolerance:  1,
		},
		{
			name:       "ZeroSpeed_GPU_Excluded",
			benchmarks: map[int]float64{0: 100.0, 1: 0.0},
			calls:      20,
			wantCounts: map[int]int{0: 20},
			tolerance:  0,
		},
		{
			name:       "NegativeSpeed_GPU_Excluded",
			benchmarks: map[int]float64{0: 100.0, 1: -50.0},
			calls:      20,
			wantCounts: map[int]int{0: 20},
			tolerance:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assigner := NewGPUAssigner(tt.benchmarks)

			counts := make(map[int]int)
			for i := 0; i < tt.calls; i++ {
				gpu := assigner.Next()
				counts[gpu]++
			}

			for gpu, want := range tt.wantCounts {
				got := counts[gpu]
				diff := got - want
				if diff < 0 {
					diff = -diff
				}
				if diff > tt.tolerance {
					t.Errorf("GPU %d: want %d±%d, got %d", gpu, want, tt.tolerance, got)
				}
			}

			for gpu, count := range counts {
				if _, expected := tt.wantCounts[gpu]; !expected && count > 0 {
					t.Errorf("unexpected GPU %d got %d calls", gpu, count)
				}
			}
		})
	}
}

func TestGPUAssigner_NumGPUs(t *testing.T) {
	tests := []struct {
		name       string
		benchmarks map[int]float64
		want       int
	}{
		{"Nil", nil, 0},
		{"Empty", map[int]float64{}, 0},
		{"OneGPU", map[int]float64{0: 100}, 1},
		{"TwoGPUs", map[int]float64{0: 100, 1: 50}, 2},
		{"ZeroExcluded", map[int]float64{0: 100, 1: 0}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewGPUAssigner(tt.benchmarks)
			got := a.NumGPUs()
			if got != tt.want {
				t.Errorf("NumGPUs() = %d, want %d", got, tt.want)
			}
		})
	}
}
