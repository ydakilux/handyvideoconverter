package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const testdataDir = "testdata"

type sample struct {
	label string
	url   string
	file  string
	size  string
}

var clips = []sample{
	{"360p  10s clip", "https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/360/Big_Buck_Bunny_360_10s_5MB.mp4", "Big_Buck_Bunny_360_10s.mp4", "~5 MB"},
	{"720p  10s clip", "https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/720/Big_Buck_Bunny_720_10s_5MB.mp4", "Big_Buck_Bunny_720_10s.mp4", "~5 MB"},
	{"1080p 10s clip", "https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/1080/Big_Buck_Bunny_1080_10s_5MB.mp4", "Big_Buck_Bunny_1080_10s.mp4", "~5 MB"},
	{"480p  full movie", "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_480p_h264.mov", "big_buck_bunny_480p.mov", "~238 MB"},
	{"720p  full movie", "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_720p_h264.mov", "big_buck_bunny_720p.mov", "~397 MB"},
	{"1080p full movie", "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_1080p_h264.mov", "big_buck_bunny_1080p.mov", "~691 MB"},
}

func main() {
	ensure := len(os.Args) > 1 && os.Args[1] == "--ensure"

	if ensure && hasTestdata() {
		return
	}

	if ensure {
		fmt.Println()
		fmt.Println("No sample videos found in testdata/.")
		fmt.Println("A sample video is needed for benchmarking and integration tests.")
		fmt.Println()
		if !isInteractive() {
			fmt.Println("Run 'make download-sample' to download one interactively.")
			return
		}
	}

	showMenuAndDownload()
}

func hasTestdata() bool {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			return true
		}
	}
	return false
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func showMenuAndDownload() {
	fmt.Println()
	fmt.Println("Big Buck Bunny - sample video downloader")
	fmt.Println("=========================================")
	fmt.Println()
	fmt.Println("  10-second clips (H.264, 60fps):")
	for i, c := range clips[:3] {
		fmt.Printf("    %d) %-6s %s\n", i+1, c.size, c.file)
	}
	fmt.Println()
	fmt.Println("  Full movie (~10 min, H.264):")
	for i, c := range clips[3:] {
		fmt.Printf("    %d) %-6s %s\n", i+4, c.size, c.file)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter choice [1-6]: ")
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(clips) {
		fmt.Fprintf(os.Stderr, "Invalid choice: %q\n", line)
		os.Exit(1)
	}

	chosen := clips[idx-1]
	dest := filepath.Join(testdataDir, chosen.file)

	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", testdataDir, err)
		os.Exit(1)
	}

	fmt.Printf("\nDownloading %s ...\n", chosen.label)
	fmt.Printf("  URL:  %s\n", chosen.url)
	fmt.Printf("  Dest: %s\n\n", dest)

	if err := download(chosen.url, dest); err != nil {
		fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
		os.Exit(1)
	}

	info, _ := os.Stat(dest)
	fmt.Printf("\nSaved: %s (%.1f MB)\n", dest, float64(info.Size())/(1024*1024))
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  reforge %s\n", testdataDir)
	fmt.Printf("  go run ./cmd/benchmark --input %s\n", testdataDir)
}

func download(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer f.Close()

	total := resp.ContentLength
	written := int64(0)
	buf := make([]byte, 64*1024)
	lastPrint := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write: %w", wErr)
			}
			written += int64(n)
			if time.Since(lastPrint) > 500*time.Millisecond {
				printProgress(written, total)
				lastPrint = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read: %w", readErr)
		}
	}

	printProgress(written, total)
	fmt.Println()
	return nil
}

func printProgress(written, total int64) {
	mb := float64(written) / (1024 * 1024)
	if total > 0 {
		pct := float64(written) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "\r  %.1f MB / %.1f MB  (%.0f%%)", mb, float64(total)/(1024*1024), pct)
	} else {
		fmt.Fprintf(os.Stderr, "\r  %.1f MB downloaded", mb)
	}
}
