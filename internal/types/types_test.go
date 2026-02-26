package types

import (
	"encoding/json"
	"testing"
)

func TestConfigJSONRoundTrip(t *testing.T) {
	original := Config{
		ServerURL:          "http://localhost:5341/",
		APIKey:             "test-key",
		UsePartialHash:     true,
		MaxQueueSize:       3,
		MediaInfoPath:      "MediaInfo.exe",
		FFmpegPath:         "ffmpeg.exe",
		FFprobePath:        "ffprobe.exe",
		TempDirectory:      "C:\\Temp",
		VideoEncoder:       "hevc_nvenc",
		QualityPreset:      "balanced",
		CustomQualitySD:    23,
		CustomQuality720p:  25,
		CustomQuality1080p: 27,
		CustomQuality4K:    30,
		FileExtensions:     []string{".MOV", ".AVI", ".MKV", ".MP4"},
		LogLevel:           "INFO",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal Config: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Config: %v", err)
	}

	if decoded.ServerURL != original.ServerURL {
		t.Errorf("ServerURL: got %q, want %q", decoded.ServerURL, original.ServerURL)
	}
	if decoded.APIKey != original.APIKey {
		t.Errorf("APIKey: got %q, want %q", decoded.APIKey, original.APIKey)
	}
	if decoded.UsePartialHash != original.UsePartialHash {
		t.Errorf("UsePartialHash: got %v, want %v", decoded.UsePartialHash, original.UsePartialHash)
	}
	if decoded.MaxQueueSize != original.MaxQueueSize {
		t.Errorf("MaxQueueSize: got %d, want %d", decoded.MaxQueueSize, original.MaxQueueSize)
	}
	if decoded.MediaInfoPath != original.MediaInfoPath {
		t.Errorf("MediaInfoPath: got %q, want %q", decoded.MediaInfoPath, original.MediaInfoPath)
	}
	if decoded.FFmpegPath != original.FFmpegPath {
		t.Errorf("FFmpegPath: got %q, want %q", decoded.FFmpegPath, original.FFmpegPath)
	}
	if decoded.FFprobePath != original.FFprobePath {
		t.Errorf("FFprobePath: got %q, want %q", decoded.FFprobePath, original.FFprobePath)
	}
	if decoded.TempDirectory != original.TempDirectory {
		t.Errorf("TempDirectory: got %q, want %q", decoded.TempDirectory, original.TempDirectory)
	}
	if decoded.VideoEncoder != original.VideoEncoder {
		t.Errorf("VideoEncoder: got %q, want %q", decoded.VideoEncoder, original.VideoEncoder)
	}
	if decoded.QualityPreset != original.QualityPreset {
		t.Errorf("QualityPreset: got %q, want %q", decoded.QualityPreset, original.QualityPreset)
	}
	if decoded.CustomQualitySD != original.CustomQualitySD {
		t.Errorf("CustomQualitySD: got %d, want %d", decoded.CustomQualitySD, original.CustomQualitySD)
	}
	if decoded.CustomQuality720p != original.CustomQuality720p {
		t.Errorf("CustomQuality720p: got %d, want %d", decoded.CustomQuality720p, original.CustomQuality720p)
	}
	if decoded.CustomQuality1080p != original.CustomQuality1080p {
		t.Errorf("CustomQuality1080p: got %d, want %d", decoded.CustomQuality1080p, original.CustomQuality1080p)
	}
	if decoded.CustomQuality4K != original.CustomQuality4K {
		t.Errorf("CustomQuality4K: got %d, want %d", decoded.CustomQuality4K, original.CustomQuality4K)
	}
	if len(decoded.FileExtensions) != len(original.FileExtensions) {
		t.Fatalf("FileExtensions length: got %d, want %d", len(decoded.FileExtensions), len(original.FileExtensions))
	}
	for i, ext := range decoded.FileExtensions {
		if ext != original.FileExtensions[i] {
			t.Errorf("FileExtensions[%d]: got %q, want %q", i, ext, original.FileExtensions[i])
		}
	}
	if decoded.LogLevel != original.LogLevel {
		t.Errorf("LogLevel: got %q, want %q", decoded.LogLevel, original.LogLevel)
	}
}

func TestConfigJSONFieldNames(t *testing.T) {
	cfg := Config{
		ServerURL:      "http://test/",
		MaxQueueSize:   5,
		UsePartialHash: true,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	expectedKeys := []string{
		"server_url", "api_key", "use_partial_hash", "max_queue_size",
		"mediainfo_path", "ffmpeg_path", "ffprobe_path", "temp_directory",
		"video_encoder", "quality_preset", "file_extensions", "log_level",
	}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q not found", key)
		}
	}
}

func TestRecordJSONRoundTrip(t *testing.T) {
	original := Record{
		OriginalSize:  1024000,
		ConvertedSize: 512000,
		Output:        "D:\\HSORTED\\test\\video.mp4",
		Note:          "already_hevc",
		Error:         "",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal Record: %v", err)
	}

	var decoded Record
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Record: %v", err)
	}

	if decoded.OriginalSize != original.OriginalSize {
		t.Errorf("OriginalSize: got %d, want %d", decoded.OriginalSize, original.OriginalSize)
	}
	if decoded.ConvertedSize != original.ConvertedSize {
		t.Errorf("ConvertedSize: got %d, want %d", decoded.ConvertedSize, original.ConvertedSize)
	}
	if decoded.Output != original.Output {
		t.Errorf("Output: got %q, want %q", decoded.Output, original.Output)
	}
	if decoded.Note != original.Note {
		t.Errorf("Note: got %q, want %q", decoded.Note, original.Note)
	}
}

func TestRecordOmitempty(t *testing.T) {
	rec := Record{
		OriginalSize: 1024,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, ok := raw["converted_size"]; ok {
		t.Error("converted_size should be omitted when zero")
	}
	if _, ok := raw["output"]; ok {
		t.Error("output should be omitted when empty")
	}
	if _, ok := raw["note"]; ok {
		t.Error("note should be omitted when empty")
	}
	if _, ok := raw["error"]; ok {
		t.Error("error should be omitted when empty")
	}
	if _, ok := raw["original_size"]; !ok {
		t.Error("original_size should always be present")
	}
}

func TestJobJSONRoundTrip(t *testing.T) {
	original := Job{
		FilePath:        "C:\\Videos\\test.mp4",
		BaseDir:         "C:\\Videos",
		DriveRoot:       "C:\\",
		FileHash:        "abc123def456",
		OriginalSize:    2048000,
		Width:           1920,
		Height:          1080,
		Format:          "HEVC",
		CodecID:         "hvc1",
		DurationSeconds: 120.5,
		FileNumber:      3,
		TotalFiles:      25,
		FolderNumber:    1,
		TotalFolders:    4,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal Job: %v", err)
	}

	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Job: %v", err)
	}

	if decoded.FilePath != original.FilePath {
		t.Errorf("FilePath: got %q, want %q", decoded.FilePath, original.FilePath)
	}
	if decoded.BaseDir != original.BaseDir {
		t.Errorf("BaseDir: got %q, want %q", decoded.BaseDir, original.BaseDir)
	}
	if decoded.DriveRoot != original.DriveRoot {
		t.Errorf("DriveRoot: got %q, want %q", decoded.DriveRoot, original.DriveRoot)
	}
	if decoded.FileHash != original.FileHash {
		t.Errorf("FileHash: got %q, want %q", decoded.FileHash, original.FileHash)
	}
	if decoded.OriginalSize != original.OriginalSize {
		t.Errorf("OriginalSize: got %d, want %d", decoded.OriginalSize, original.OriginalSize)
	}
	if decoded.Width != original.Width {
		t.Errorf("Width: got %d, want %d", decoded.Width, original.Width)
	}
	if decoded.Height != original.Height {
		t.Errorf("Height: got %d, want %d", decoded.Height, original.Height)
	}
	if decoded.Format != original.Format {
		t.Errorf("Format: got %q, want %q", decoded.Format, original.Format)
	}
	if decoded.CodecID != original.CodecID {
		t.Errorf("CodecID: got %q, want %q", decoded.CodecID, original.CodecID)
	}
	if decoded.DurationSeconds != original.DurationSeconds {
		t.Errorf("DurationSeconds: got %f, want %f", decoded.DurationSeconds, original.DurationSeconds)
	}
	if decoded.FileNumber != original.FileNumber {
		t.Errorf("FileNumber: got %d, want %d", decoded.FileNumber, original.FileNumber)
	}
	if decoded.TotalFiles != original.TotalFiles {
		t.Errorf("TotalFiles: got %d, want %d", decoded.TotalFiles, original.TotalFiles)
	}
	if decoded.FolderNumber != original.FolderNumber {
		t.Errorf("FolderNumber: got %d, want %d", decoded.FolderNumber, original.FolderNumber)
	}
	if decoded.TotalFolders != original.TotalFolders {
		t.Errorf("TotalFolders: got %d, want %d", decoded.TotalFolders, original.TotalFolders)
	}
}
