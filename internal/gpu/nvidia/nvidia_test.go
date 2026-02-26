package nvidia

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	return l
}

func TestIsAvailable_NoPanic(t *testing.T) {
	got := IsAvailable()
	t.Logf("IsAvailable() = %v", got)
}

func TestParseGPULine_SingleGPU(t *testing.T) {
	line := "0, NVIDIA GeForce RTX 4090, 24564, 1234, 23330"
	gpu, err := parseGPULine(line)
	if err != nil {
		t.Fatalf("parseGPULine(%q) returned error: %v", line, err)
	}

	if gpu.Index != 0 {
		t.Errorf("Index = %d, want 0", gpu.Index)
	}
	if gpu.Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("Name = %q, want %q", gpu.Name, "NVIDIA GeForce RTX 4090")
	}
	if gpu.VRAMTotalMB != 24564 {
		t.Errorf("VRAMTotalMB = %d, want 24564", gpu.VRAMTotalMB)
	}
	if gpu.VRAMUsedMB != 1234 {
		t.Errorf("VRAMUsedMB = %d, want 1234", gpu.VRAMUsedMB)
	}
	if gpu.VRAMFreeMB != 23330 {
		t.Errorf("VRAMFreeMB = %d, want 23330", gpu.VRAMFreeMB)
	}
}

func TestParseGPUList_MultiGPU(t *testing.T) {
	output := "0, NVIDIA GeForce RTX 4090, 24564, 1234, 23330\n1, NVIDIA GeForce RTX 3080, 10240, 512, 9728"
	gpus, err := parseGPUList(output)
	if err != nil {
		t.Fatalf("parseGPUList returned error: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs, want 2", len(gpus))
	}
	if gpus[0].Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("GPU[0].Name = %q, want %q", gpus[0].Name, "NVIDIA GeForce RTX 4090")
	}
	if gpus[1].Index != 1 {
		t.Errorf("GPU[1].Index = %d, want 1", gpus[1].Index)
	}
	if gpus[1].VRAMTotalMB != 10240 {
		t.Errorf("GPU[1].VRAMTotalMB = %d, want 10240", gpus[1].VRAMTotalMB)
	}
}

func TestParseGPUList_EmptyOutput(t *testing.T) {
	gpus, err := parseGPUList("")
	if err != nil {
		t.Fatalf("parseGPUList(\"\") returned error: %v", err)
	}
	if len(gpus) != 0 {
		t.Errorf("got %d GPUs, want 0", len(gpus))
	}
}

func TestParseGPULine_InvalidFieldCount(t *testing.T) {
	_, err := parseGPULine("0, NVIDIA GeForce RTX 4090, 24564")
	if err == nil {
		t.Error("expected error for line with wrong field count, got nil")
	}
}

func TestParseGPULine_InvalidNumber(t *testing.T) {
	_, err := parseGPULine("0, NVIDIA GeForce RTX 4090, not_a_number, 1234, 23330")
	if err == nil {
		t.Error("expected error for non-numeric VRAM total, got nil")
	}
}

func TestQueryGPUs_ReturnsEmptySliceWhenUnavailable(t *testing.T) {
	if IsAvailable() {
		t.Skip("nvidia-smi is available; this test checks behavior when absent")
	}
	logger := testLogger()
	gpus, err := QueryGPUs(logger)
	if err != nil {
		t.Fatalf("QueryGPUs returned error: %v", err)
	}
	if len(gpus) != 0 {
		t.Errorf("got %d GPUs, want 0 when nvidia-smi absent", len(gpus))
	}
}

func TestQuerySessionCount_ReturnsZeroWhenUnavailable(t *testing.T) {
	if IsAvailable() {
		t.Skip("nvidia-smi is available; this test checks behavior when absent")
	}
	logger := testLogger()
	count, err := QuerySessionCount(0, logger)
	if err != nil {
		t.Fatalf("QuerySessionCount returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("got count %d, want 0 when nvidia-smi absent", count)
	}
}
