// Package main provides a cross-platform benchmark tool that runs the video
// converter multiple times with varying --jobs values and records elapsed time.
//
// Usage:
//
//	go run ./cmd/benchmark --input D:\Videos\ --jobs 1,2,4,8
//	go run ./cmd/benchmark --input D:\Videos\ --jobs 1,2,4 --output results.csv
//	go run ./cmd/benchmark --input D:\Videos\ --jobs 1,2,4 --bin ./video-converter.exe
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"video-converter/internal/config"
	"video-converter/internal/fileutil"
)

type result struct {
	jobs    int
	elapsed string
	wallMs  int64
	err     string
}

func main() {
	inputDir := flag.String("input", "", "Input directory to convert (required)")
	jobsFlag := flag.String("jobs", "1,2,4,8", "Comma-separated list of --jobs values to test")
	outputCSV := flag.String("output", "benchmark_results.csv", "Path to write CSV results")
	binPath := flag.String("bin", "", "Path to video-converter binary (default: auto-detect)")
	extraFlags := flag.String("extra-flags", "", "Additional flags to pass to the converter")
	flag.Parse()

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --input is required")
		flag.Usage()
		os.Exit(1)
	}

	bin, err := resolveBinary(*binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Binary: %s\n", bin)

	jobValues, err := parseJobsList(*jobsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid --jobs value: %v\n", err)
		os.Exit(1)
	}

	results := make([]result, 0, len(jobValues))

	fmt.Printf("Benchmarking %d job configuration(s) against: %s\n\n", len(jobValues), *inputDir)

	for _, jobs := range jobValues {
		fmt.Printf("▶  jobs=%d ... ", jobs)

		args := []string{
			"--bypass",
			"--force-hevc",
			"--same-drive",
			"--non-interactive",
			fmt.Sprintf("--jobs=%d", jobs),
		}
		if *extraFlags != "" {
			args = append(args, strings.Fields(*extraFlags)...)
		}
		args = append(args, *inputDir)

		start := time.Now()
		elapsed, runErr := runConverter(bin, args)
		wall := time.Since(start).Milliseconds()

		r := result{jobs: jobs, wallMs: wall}
		if runErr != nil {
			r.err = runErr.Error()
			r.elapsed = fmt.Sprintf("wall %s", fmtMs(wall))
			fmt.Printf("FAILED (%v)  wall=%s\n", runErr, fmtMs(wall))
		} else {
			r.elapsed = elapsed
			fmt.Printf("ELAPSED=%s  wall=%s\n", elapsed, fmtMs(wall))
		}
		results = append(results, r)
	}

	// Print summary table
	fmt.Println()
	fmt.Println("┌────────┬──────────────┬──────────────┐")
	fmt.Println("│  jobs  │   elapsed    │  wall time   │")
	fmt.Println("├────────┼──────────────┼──────────────┤")
	for _, r := range results {
		status := r.elapsed
		if r.err != "" {
			status = "ERROR"
		}
		fmt.Printf("│ %-6d │ %-12s │ %-12s │\n", r.jobs, status, fmtMs(r.wallMs))
	}
	fmt.Println("└────────┴──────────────┴──────────────┘")

	// Write CSV
	if err := writeCSV(*outputCSV, results); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to write CSV: %v\n", err)
	} else {
		fmt.Printf("\nResults written to: %s\n", *outputCSV)
	}
}

// resolveBinary finds the video-converter binary to use.
func resolveBinary(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("invalid --bin path: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("binary not found at %s: %w", abs, err)
		}
		return abs, nil
	}

	// Try exe name based on platform
	exeName := config.ExeName("video-converter")

	// 1. Check same directory as this process's executable
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), exeName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 2. Check current working directory
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, exeName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 3. Search PATH
	if path, err := exec.LookPath(exeName); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not find %s — use --bin to specify path", exeName)
}

// parseJobsList parses "1,2,4,8" into []int.
func parseJobsList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("%q is not an integer", p)
		}
		if n < 1 {
			return nil, fmt.Errorf("jobs value must be >= 1, got %d", n)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty jobs list")
	}
	return out, nil
}

// runConverter executes the converter binary with args, captures stdout,
// and parses the "ELAPSED=<value>" line that --non-interactive prints.
func runConverter(bin string, args []string) (elapsed string, err error) {
	cmd := exec.Command(bin, args...)

	// Pipe stdout so we can parse ELAPSED= while also echoing to terminal
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("pipe stdout: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ELAPSED=") {
			elapsed = strings.TrimPrefix(line, "ELAPSED=")
		}
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("exit: %w", err)
	}

	if elapsed == "" {
		return "(not reported)", nil
	}
	return elapsed, nil
}

func fmtMs(ms int64) string {
	return fileutil.FmtElapsed(time.Duration(ms) * time.Millisecond)
}

// writeCSV writes benchmark results to a CSV file.
func writeCSV(path string, results []result) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"jobs", "elapsed", "wall_ms", "error"}); err != nil {
		return err
	}
	for _, r := range results {
		if err := w.Write([]string{
			strconv.Itoa(r.jobs),
			r.elapsed,
			strconv.FormatInt(r.wallMs, 10),
			r.err,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
