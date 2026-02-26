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

func Run(ffmpegExe string, args []string, filePath string, totalDuration float64, logger *logrus.Logger) (int, string) {
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
					if pct > lastPct && (pct%10 == 0 || pct == 100) {
						logger.Infof("Progress: %d%%", pct)
						lastPct = pct
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

func GetMediaInfo(filePath, mediaInfoExe string) (*types.VideoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, mediaInfoExe, "--output=JSON", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Media struct {
			Track []map[string]interface{} `json:"track"`
		} `json:"media"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	for _, track := range result.Media.Track {
		if trackType, ok := track["@type"].(string); ok && trackType == "Video" {
			info := &types.VideoInfo{}

			if format, ok := track["Format"].(string); ok {
				info.Format = format
			}
			if codecID, ok := track["CodecID"].(string); ok {
				info.CodecID = codecID
			}
			if width, ok := track["Width"].(string); ok {
				fmt.Sscanf(width, "%d", &info.Width)
			}
			if height, ok := track["Height"].(string); ok {
				fmt.Sscanf(height, "%d", &info.Height)
			}

			return info, nil
		}
	}

	return nil, fmt.Errorf("no video track found")
}

func IsHEVC(format, codecID string) bool {
	return strings.Contains(strings.ToUpper(format), "HEVC") ||
		strings.Contains(strings.ToUpper(codecID), "HVC1")
}
