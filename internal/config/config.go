package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// ResolveExecutable resolves the path to an executable. It first checks the
// config-supplied path (absolute or relative to execDir); if not found or empty,
// it falls back to exec.LookPath.
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
