package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"video-converter/internal/types"
)

func BuildArgs(inputPath, outputPath, quality, outputExt, encoder string) []string {
	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-progress", "pipe:1",
		"-i", inputPath,
		"-c:v", encoder,
	}

	if encoder == "libx265" {
		args = append(args, "-crf", quality, "-preset", "medium")
	} else if strings.Contains(encoder, "_nvenc") {
		args = append(args, "-cq", quality, "-preset", "p5")
	} else {
		args = append(args, "-crf", quality)
	}

	if outputExt == ".mkv" {
		args = append(args,
			"-c:a", "copy",
			"-c:s", "copy",
			"-map", "0",
			"-map_metadata", "0",
			"-map_chapters", "0",
		)
	} else {
		args = append(args,
			"-c:a", "aac",
			"-movflags", "+faststart",
		)
	}

	args = append(args, outputPath)

	return args
}

// Run executes FFmpeg with the given args. onProgress is called with the
// percentage (0-100) each time it changes; pass nil to disable callbacks.
func Run(ffmpegExe string, args []string, filePath string, totalDuration float64, logger *logrus.Logger, onProgress func(pct int)) (int, string) {
	cmd := exec.Command(ffmpegExe, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Errorf("Failed to create stdout pipe: %v", err)
		return 999, ""
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Errorf("Failed to create stderr pipe: %v", err)
		return 999, ""
	}

	if err := cmd.Start(); err != nil {
		logger.Errorf("Failed to start FFmpeg: %v", err)
		return 999, ""
	}

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		stderrBuf.ReadFrom(stderr)
	}()

	lastPct := -1
	var lastSent time.Time
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_ms=") || strings.HasPrefix(line, "out_time_us=") || strings.HasPrefix(line, "out_time=") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				var outTime float64
				if strings.HasPrefix(line, "out_time_ms=") {
					if val, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						outTime = float64(val) / 1000000.0
					}
				} else if strings.HasPrefix(line, "out_time_us=") {
					if val, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						outTime = float64(val) / 1000000.0
					}
				} else if strings.HasPrefix(line, "out_time=") {
					timeParts := strings.Split(parts[1], ":")
					if len(timeParts) == 3 {
						h, _ := strconv.Atoi(timeParts[0])
						m, _ := strconv.Atoi(timeParts[1])
						s, _ := strconv.ParseFloat(timeParts[2], 64)
						outTime = float64(h*3600+m*60) + s
					}
				}

				if totalDuration > 0 && outTime > 0 {
					pct := int(100 * outTime / totalDuration)
					if pct > lastPct {
						lastPct = pct
						if onProgress != nil {
							now := time.Now()
							if pct >= 100 || lastSent.IsZero() || now.Sub(lastSent) >= 2500*time.Millisecond {
								lastSent = now
								onProgress(pct)
							}
						}
					}
				}
			}
		}
	}

	<-stderrDone

	if err := cmd.Wait(); err != nil {
		stderrStr := stderrBuf.String()
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), stderrStr
		}
		killProcess(cmd)
		return 999, stderrStr
	}

	return 0, stderrBuf.String()
}

func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.Run()
}

func GetDuration(filePath, ffprobeExe string, logger *logrus.Logger) float64 {
	if ffprobeExe == "" {
		return 0.0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobeExe, "-v", "error", "-show_entries", "format=duration", "-of", "json", filePath)
	output, err := cmd.Output()
	if err != nil {
		return 0.0
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return 0.0
	}

	duration, _ := strconv.ParseFloat(result.Format.Duration, 64)
	return duration
}

func GetMediaInfo(filePath, ffprobeExe string) (*types.VideoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobeExe,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v:0",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var result struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			CodecTag  string `json:"codec_tag_string"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	if len(result.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found")
	}

	s := result.Streams[0]
	return &types.VideoInfo{
		Format:  s.CodecName,
		CodecID: s.CodecTag,
		Width:   s.Width,
		Height:  s.Height,
	}, nil
}

func IsHEVC(format, codecID string) bool {
	return strings.Contains(strings.ToUpper(format), "HEVC") ||
		strings.Contains(strings.ToUpper(codecID), "HVC1")
}
