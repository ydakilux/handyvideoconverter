// Package nvidia queries nvidia-smi for GPU metadata (VRAM, driver version,
// NVENC session counts) used by the benchmark and distributor subsystems.
package nvidia

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// NvidiaGPU holds metadata for a single NVIDIA GPU queried via nvidia-smi.
type NvidiaGPU struct {
	Index               int
	Name                string
	VRAMTotalMB         int
	VRAMUsedMB          int
	VRAMFreeMB          int
	EncoderSessionCount int
	EncoderUtilPct      int
	GPUUtilPct          int
}

// IsAvailable checks whether nvidia-smi exists in PATH.
func IsAvailable() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

// QueryGPUs runs nvidia-smi to enumerate all NVIDIA GPUs and their VRAM info.
// Returns an empty slice (not an error) when nvidia-smi is absent or fails.
func QueryGPUs(logger *logrus.Logger) ([]NvidiaGPU, error) {
	if !IsAvailable() {
		logger.Debug("nvidia-smi not found in PATH, skipping NVIDIA GPU query")
		return []NvidiaGPU{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.used,memory.free",
		"--format=csv,noheader,nounits",
	)
	output, err := cmd.Output()
	if err != nil {
		logger.Warnf("nvidia-smi query failed: %v", err)
		return []NvidiaGPU{}, nil
	}

	return parseGPUList(strings.TrimSpace(string(output)))
}

// QuerySessionCount queries the active NVENC encoder session count for a specific GPU.
// Returns 0 (not an error) when nvidia-smi is absent or fails.
func QuerySessionCount(gpuIndex int, logger *logrus.Logger) (int, error) {
	if !IsAvailable() {
		logger.Debug("nvidia-smi not found in PATH, returning session count 0")
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=encoder.stats.sessionCount",
		"--format=csv,noheader,nounits",
		"-i", strconv.Itoa(gpuIndex),
	)
	output, err := cmd.Output()
	if err != nil {
		logger.Warnf("nvidia-smi session count query failed for GPU %d: %v", gpuIndex, err)
		return 0, nil
	}

	countStr := strings.TrimSpace(string(output))
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse session count %q: %w", countStr, err)
	}

	return count, nil
}

// parseGPUList parses multi-line CSV output from nvidia-smi into NvidiaGPU structs.
func parseGPUList(output string) ([]NvidiaGPU, error) {
	if output == "" {
		return []NvidiaGPU{}, nil
	}

	lines := strings.Split(output, "\n")
	gpus := make([]NvidiaGPU, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		gpu, err := parseGPULine(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GPU line %q: %w", line, err)
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

// parseGPULine parses a single CSV line from nvidia-smi query output.
// Expected format: "0, NVIDIA GeForce RTX 4090, 24564, 1234, 23330"
func parseGPULine(line string) (NvidiaGPU, error) {
	fields := strings.Split(line, ", ")
	if len(fields) != 5 {
		return NvidiaGPU{}, fmt.Errorf("expected 5 fields, got %d in %q", len(fields), line)
	}

	index, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return NvidiaGPU{}, fmt.Errorf("invalid index %q: %w", fields[0], err)
	}

	name := strings.TrimSpace(fields[1])

	vramTotal, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil {
		return NvidiaGPU{}, fmt.Errorf("invalid VRAM total %q: %w", fields[2], err)
	}

	vramUsed, err := strconv.Atoi(strings.TrimSpace(fields[3]))
	if err != nil {
		return NvidiaGPU{}, fmt.Errorf("invalid VRAM used %q: %w", fields[3], err)
	}

	vramFree, err := strconv.Atoi(strings.TrimSpace(fields[4]))
	if err != nil {
		return NvidiaGPU{}, fmt.Errorf("invalid VRAM free %q: %w", fields[4], err)
	}

	return NvidiaGPU{
		Index:       index,
		Name:        name,
		VRAMTotalMB: vramTotal,
		VRAMUsedMB:  vramUsed,
		VRAMFreeMB:  vramFree,
	}, nil
}
