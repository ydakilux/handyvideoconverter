package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"video-converter/internal/types"
)

var ValidEncoders = []string{"auto", "hevc_nvenc", "hevc_amf", "hevc_qsv", "libx265"}

func LoadConfig(path string) (types.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return CreateDefaultConfig(path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return types.Config{}, err
	}
	var cfg types.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return types.Config{}, err
	}
	if err := ValidateEncoder(cfg.VideoEncoder); err != nil {
		return types.Config{}, err
	}
	return cfg, nil
}

func CreateDefaultConfig(path string) (types.Config, error) {
	cfg := types.Config{
		ServerURL:          "http://localhost:5341/",
		APIKey:             "",
		UsePartialHash:     true,
		MaxQueueSize:       3,
		MaxParallelJobs:    1,
		MediaInfoPath:      "MediaInfo_CLI_24.04_Windows_x64\\MediaInfo.exe",
		FFmpegPath:         "ffmpeg\\bin\\ffmpeg.exe",
		FFprobePath:        "",
		TempDirectory:      "",
		VideoEncoder:       "hevc_nvenc",
		QualityPreset:      "balanced",
		CustomQualitySD:    0,
		CustomQuality720p:  0,
		CustomQuality1080p: 0,
		CustomQuality4K:    0,
		FileExtensions:     []string{".MOV", ".AVI", ".MKV", ".MP4", ".WMV", ".M4V", ".FLV", ".MPG", ".ASF", ".TS", ".M2TS"},
		LogLevel:           "INFO",
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return types.Config{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return types.Config{}, err
	}
	return cfg, nil
}

func ValidateEncoder(encoder string) error {
	if encoder == "" {
		return nil
	}
	for _, valid := range ValidEncoders {
		if encoder == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid video encoder %q; valid options: %s", encoder, strings.Join(ValidEncoders, ", "))
}

func DetermineQuality(width int, cfg types.Config) string {
	if cfg.CustomQualitySD > 0 && width <= 1024 {
		return strconv.Itoa(cfg.CustomQualitySD)
	}
	if cfg.CustomQuality720p > 0 && width <= 1280 {
		return strconv.Itoa(cfg.CustomQuality720p)
	}
	if cfg.CustomQuality1080p > 0 && width <= 1920 {
		return strconv.Itoa(cfg.CustomQuality1080p)
	}
	if cfg.CustomQuality4K > 0 && width > 1920 {
		return strconv.Itoa(cfg.CustomQuality4K)
	}

	switch strings.ToLower(cfg.QualityPreset) {
	case "high_quality":
		if width <= 1024 {
			return "19"
		} else if width <= 1280 {
			return "20"
		} else if width <= 1920 {
			return "21"
		}
		return "23"

	case "balanced":
		if width <= 1024 {
			return "23"
		} else if width <= 1280 {
			return "25"
		} else if width <= 1920 {
			return "27"
		}
		return "30"

	case "space_saver":
		if width <= 1024 {
			return "26"
		} else if width <= 1280 {
			return "28"
		} else if width <= 1920 {
			return "30"
		}
		return "33"

	default:
		if width <= 1024 {
			return "23"
		} else if width <= 1280 {
			return "25"
		} else if width <= 1920 {
			return "27"
		}
		return "30"
	}
}

func ResolveExecutable(configPath, exeName, execDir string) string {
	if configPath == "" {
		exe, _ := exec.LookPath(exeName)
		return exe
	}

	var path string
	if filepath.IsAbs(configPath) {
		path = configPath
	} else {
		path = filepath.Join(execDir, configPath)
	}

	if _, err := os.Stat(path); err == nil {
		return path
	}

	exe, _ := exec.LookPath(exeName)
	return exe
}

// ApplyGPUDefaults sets default values for GPU-related config fields.
// Only sets defaults when fields are at their zero value.
func ApplyGPUDefaults(cfg *types.Config) {
	if cfg.MaxEncodesPerGPU == 0 {
		cfg.MaxEncodesPerGPU = 2
	}
	if cfg.MaxParallelJobs == 0 {
		cfg.MaxParallelJobs = 1
	}
}
