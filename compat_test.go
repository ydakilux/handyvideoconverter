package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"github.com/sirupsen/logrus"
	cfgpkg "video-converter/internal/config"
	"video-converter/internal/database"
	"video-converter/internal/encoder"
	"video-converter/internal/ffmpeg"
	"video-converter/internal/types"
)

// ---------------------------------------------------------------------------
// Test 1: Old-format config (no GPU fields) loads cleanly
// ---------------------------------------------------------------------------

func TestBackwardCompatConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "old_config.json")

	// Minimal old-format JSON — only the original fields, NO GPU fields
	oldJSON := `{
  "server_url": "http://myseq:5341/",
  "api_key": "old-key-123",
  "use_partial_hash": true,
  "max_queue_size": 5,
  "mediainfo_path": "MediaInfo\\MediaInfo.exe",
  "ffmpeg_path": "ffmpeg\\bin\\ffmpeg.exe",
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced",
  "file_extensions": [".MOV", ".AVI", ".MKV", ".MP4"],
  "log_level": "DEBUG"
}`
	if err := os.WriteFile(cfgPath, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	cfg, err := cfgpkg.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig with old-format JSON must succeed: %v", err)
	}

	// Verify all original fields parsed correctly
	if cfg.ServerURL != "http://myseq:5341/" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "http://myseq:5341/")
	}
	if cfg.APIKey != "old-key-123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "old-key-123")
	}
	if !cfg.UsePartialHash {
		t.Error("UsePartialHash should be true")
	}
	if cfg.MaxQueueSize != 5 {
		t.Errorf("MaxQueueSize = %d, want 5", cfg.MaxQueueSize)
	}
	if cfg.MediaInfoPath != "MediaInfo\\MediaInfo.exe" {
		t.Errorf("MediaInfoPath = %q, want %q", cfg.MediaInfoPath, "MediaInfo\\MediaInfo.exe")
	}
	if cfg.FFmpegPath != "ffmpeg\\bin\\ffmpeg.exe" {
		t.Errorf("FFmpegPath = %q, want %q", cfg.FFmpegPath, "ffmpeg\\bin\\ffmpeg.exe")
	}
	if cfg.VideoEncoder != "hevc_nvenc" {
		t.Errorf("VideoEncoder = %q, want %q", cfg.VideoEncoder, "hevc_nvenc")
	}
	if cfg.QualityPreset != "balanced" {
		t.Errorf("QualityPreset = %q, want %q", cfg.QualityPreset, "balanced")
	}
	wantExts := []string{".MOV", ".AVI", ".MKV", ".MP4"}
	if !reflect.DeepEqual(cfg.FileExtensions, wantExts) {
		t.Errorf("FileExtensions = %v, want %v", cfg.FileExtensions, wantExts)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "DEBUG")
	}

	// GPU fields must have zero values (not errors)
	if cfg.BenchmarkCache != nil {
		t.Errorf("BenchmarkCache should be nil, got %v", cfg.BenchmarkCache)
	}
	if cfg.MaxEncodesPerGPU != 0 {
		t.Errorf("MaxEncodesPerGPU = %d, want 0", cfg.MaxEncodesPerGPU)
	}
	if cfg.NonInteractive {
		t.Error("NonInteractive should be false")
	}
	if cfg.GPUPreset != "" {
		t.Errorf("GPUPreset = %q, want empty", cfg.GPUPreset)
	}
	if cfg.Rebenchmark {
		t.Error("Rebenchmark should be false")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Various encoder values load without validation error
// ---------------------------------------------------------------------------

func TestBackwardCompatAutoEncoder(t *testing.T) {
	cases := []struct {
		name    string
		encoder string
	}{
		{"auto", "auto"},
		{"hevc_nvenc", "hevc_nvenc"},
		{"libx265", "libx265"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "cfg.json")

			cfg := types.Config{
				VideoEncoder:   tc.encoder,
				FileExtensions: []string{".MP4"},
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				t.Fatalf("write: %v", err)
			}

			loaded, err := cfgpkg.LoadConfig(cfgPath)
			if err != nil {
				t.Fatalf("LoadConfig with encoder=%q must succeed: %v", tc.encoder, err)
			}
			if loaded.VideoEncoder != tc.encoder {
				t.Errorf("VideoEncoder = %q, want %q", loaded.VideoEncoder, tc.encoder)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 3: FFmpeg argument equivalence for libx265 between old and new paths
// ---------------------------------------------------------------------------

func TestBackwardCompatFFmpegArgs(t *testing.T) {
	libx265Enc := encoder.NewLibx265Encoder()

	cases := []struct {
		name      string
		preset    string
		width     int
		outputExt string
	}{
		{"balanced_1080p_mp4", "balanced", 1920, ".mp4"},
		{"balanced_1080p_mkv", "balanced", 1920, ".mkv"},
		{"high_quality_720p_mp4", "high_quality", 1280, ".mp4"},
		{"space_saver_sd_mp4", "space_saver", 720, ".mp4"},
	}

	input := `C:\Videos\test.avi`
	output := `C:\HSORTED\test\test.mp4`

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// OLD path: DetermineQuality → BuildArgs (always uses -preset medium)
			cfg := types.Config{QualityPreset: tc.preset}
			quality := cfgpkg.DetermineQuality(tc.width, cfg)

			outFile := output
			if tc.outputExt == ".mkv" {
				outFile = `C:\HSORTED\test\test.mkv`
			}
			oldArgs := ffmpeg.BuildArgs(input, outFile, quality, tc.outputExt, "libx265")

			// NEW path: encoder.QualityArgs → buildConversionArgs
			qualityArgs := libx265Enc.QualityArgs(tc.preset, tc.width)
			newArgs := buildConversionArgs(input, outFile, tc.outputExt, "libx265", qualityArgs, nil)

			// For "balanced" preset, old (-preset medium) == new (-preset medium) → identical
			// For "high_quality", old used -preset medium but new uses -preset slow (intentional improvement)
			// For "space_saver", old used -preset medium but new uses -preset faster (intentional improvement)
			//
			// We verify:
			//  a) CRF values are always identical between old and new paths
			//  b) For balanced, the full arg lists are identical
			//  c) For non-balanced, only the -preset value differs (documented change)

			oldCRF := extractArgValue(oldArgs, "-crf")
			newCRF := extractArgValue(newArgs, "-crf")
			if oldCRF != newCRF {
				t.Errorf("CRF mismatch: old=%q new=%q", oldCRF, newCRF)
			}

			if tc.preset == "balanced" {
				// Full equivalence for balanced preset
				if !reflect.DeepEqual(oldArgs, newArgs) {
					t.Errorf("balanced args should be identical\nold: %v\nnew: %v", oldArgs, newArgs)
				}
			} else {
				// Non-balanced: only -preset differs, all other args must match
				oldPreset := extractArgValue(oldArgs, "-preset")
				newPreset := extractArgValue(newArgs, "-preset")

				if oldPreset != "medium" {
					t.Errorf("old path should always use -preset medium, got %q", oldPreset)
				}

				// Verify the intentional preset change is documented correctly
				switch tc.preset {
				case "high_quality":
					if newPreset != "slow" {
						t.Errorf("new high_quality should use -preset slow, got %q", newPreset)
					}
				case "space_saver":
					if newPreset != "faster" {
						t.Errorf("new space_saver should use -preset faster, got %q", newPreset)
					}
				}

				// Strip -preset from both and compare everything else
				oldStripped := removeArgPair(oldArgs, "-preset")
				newStripped := removeArgPair(newArgs, "-preset")
				if !reflect.DeepEqual(oldStripped, newStripped) {
					t.Errorf("args differ beyond -preset\nold (stripped): %v\nnew (stripped): %v", oldStripped, newStripped)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 4: Old database JSON format loads via DatabaseManager
// ---------------------------------------------------------------------------

func TestBackwardCompatDatabase(t *testing.T) {
	dir := t.TempDir()

	// Write old-format database JSON to temp directory
	dbJSON := `{
  "hashkey1": {
    "original_size": 1000,
    "converted_size": 500,
    "output": "D:\\HSORTED\\test\\file.mp4"
  },
  "hashkey2": {
    "original_size": 2000,
    "note": "not_beneficial"
  },
  "hashkey3": {
    "original_size": 3000,
    "error": "rc_1"
  }
}`
	dbPath := filepath.Join(dir, "converted_files.json")
	if err := os.WriteFile(dbPath, []byte(dbJSON), 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	// DatabaseManager uses driveRoot + "converted_files.json"
	// So driveRoot = dir (which includes a trailing separator via TempDir)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	dbMgr := database.NewDatabaseManager(logger)

	// Use dir as drive root — DatabaseManager will look for dir/converted_files.json
	driveRoot := dir + string(filepath.Separator)

	// Record 1: successful conversion
	rec1 := dbMgr.GetRecord(driveRoot, "hashkey1")
	if rec1 == nil {
		t.Fatal("GetRecord(hashkey1) returned nil")
	}
	if rec1.OriginalSize != 1000 {
		t.Errorf("OriginalSize = %d, want 1000", rec1.OriginalSize)
	}
	if rec1.ConvertedSize != 500 {
		t.Errorf("ConvertedSize = %d, want 500", rec1.ConvertedSize)
	}
	if rec1.Output != `D:\HSORTED\test\file.mp4` {
		t.Errorf("Output = %q, want %q", rec1.Output, `D:\HSORTED\test\file.mp4`)
	}

	// Record 2: not beneficial
	rec2 := dbMgr.GetRecord(driveRoot, "hashkey2")
	if rec2 == nil {
		t.Fatal("GetRecord(hashkey2) returned nil")
	}
	if rec2.OriginalSize != 2000 {
		t.Errorf("OriginalSize = %d, want 2000", rec2.OriginalSize)
	}
	if rec2.Note != "not_beneficial" {
		t.Errorf("Note = %q, want %q", rec2.Note, "not_beneficial")
	}

	// Record 3: error
	rec3 := dbMgr.GetRecord(driveRoot, "hashkey3")
	if rec3 == nil {
		t.Fatal("GetRecord(hashkey3) returned nil")
	}
	if rec3.Error != "rc_1" {
		t.Errorf("Error = %q, want %q", rec3.Error, "rc_1")
	}

	// Non-existent key
	recNil := dbMgr.GetRecord(driveRoot, "nonexistent")
	if recNil != nil {
		t.Errorf("GetRecord(nonexistent) should return nil, got %+v", recNil)
	}

	// Verify UpdateRecord round-trips correctly
	dbMgr.UpdateRecord(driveRoot, "hashkey4", types.Record{
		OriginalSize:  4000,
		ConvertedSize: 2000,
		Output:        `D:\HSORTED\new\video.mp4`,
	})
	rec4 := dbMgr.GetRecord(driveRoot, "hashkey4")
	if rec4 == nil {
		t.Fatal("GetRecord(hashkey4) returned nil after UpdateRecord")
	}
	if rec4.OriginalSize != 4000 || rec4.ConvertedSize != 2000 {
		t.Errorf("UpdateRecord round-trip failed: got %+v", rec4)
	}
}

// ---------------------------------------------------------------------------
// Test 5: CRF quality values match between DetermineQuality and libx265 QualityArgs
// ---------------------------------------------------------------------------

func TestBackwardCompatQualityValues(t *testing.T) {
	libx265Enc := encoder.NewLibx265Encoder()

	presets := []string{"balanced", "high_quality", "space_saver"}
	widths := []int{720, 1024, 1280, 1920, 3840}

	for _, preset := range presets {
		for _, width := range widths {
			t.Run(preset+"_"+itoa(width), func(t *testing.T) {
				// OLD path: DetermineQuality returns CRF string
				cfg := types.Config{QualityPreset: preset}
				oldCRF := cfgpkg.DetermineQuality(width, cfg)

				// NEW path: QualityArgs returns ["-crf", crf, "-preset", preset]
				qualityArgs := libx265Enc.QualityArgs(preset, width)
				newCRF := extractArgValue(qualityArgs, "-crf")

				if oldCRF != newCRF {
					t.Errorf("CRF mismatch for preset=%q width=%d: old=%q new=%q",
						preset, width, oldCRF, newCRF)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractArgValue returns the value following the given flag in an arg slice.
// Returns empty string if flag not found.
func extractArgValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// removeArgPair removes a flag and its value from an arg slice.
func removeArgPair(args []string, flag string) []string {
	var result []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == flag {
			skip = true
			continue
		}
		result = append(result, a)
	}
	return result
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
