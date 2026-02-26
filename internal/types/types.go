package types

import (
	"sync"
	"time"
)

// Config holds application configuration
type Config struct {
	ServerURL          string   `json:"server_url"`
	APIKey             string   `json:"api_key"`
	UsePartialHash     bool     `json:"use_partial_hash"`
	MaxQueueSize       int      `json:"max_queue_size"`
	MediaInfoPath      string   `json:"mediainfo_path"`
	FFmpegPath         string   `json:"ffmpeg_path"`
	FFprobePath        string   `json:"ffprobe_path"`
	TempDirectory      string   `json:"temp_directory"`
	VideoEncoder       string   `json:"video_encoder"`
	QualityPreset      string   `json:"quality_preset"`
	CustomQualitySD    int      `json:"custom_quality_sd,omitempty"`
	CustomQuality720p  int      `json:"custom_quality_720p,omitempty"`
	CustomQuality1080p int      `json:"custom_quality_1080p,omitempty"`
	CustomQuality4K    int      `json:"custom_quality_4k,omitempty"`
	FileExtensions     []string `json:"file_extensions"`
	LogLevel           string   `json:"log_level"`
	// GPU-related fields
	BenchmarkCache   map[string]BenchmarkCacheEntry `json:"benchmark_cache,omitempty"`
	MaxEncodesPerGPU int                            `json:"max_encodes_per_gpu,omitempty"`
	NonInteractive   bool                           `json:"non_interactive,omitempty"`
	GPUPreset        string                         `json:"gpu_preset,omitempty"`
	Rebenchmark      bool                           `json:"-"` // runtime-only, NOT persisted
}

// BenchmarkCacheEntry stores GPU benchmark results
type BenchmarkCacheEntry struct {
	FPS         float64   `json:"fps"`
	Timestamp   time.Time `json:"timestamp"`
	EncoderName string    `json:"encoder_name"`
}

// Record represents a cache entry
type Record struct {
	OriginalSize  int64  `json:"original_size"`
	ConvertedSize int64  `json:"converted_size,omitempty"`
	Output        string `json:"output,omitempty"`
	Note          string `json:"note,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Job represents a conversion job
type Job struct {
	FilePath        string
	BaseDir         string // User-provided input directory (dropped folder)
	DriveRoot       string
	FileHash        string
	OriginalSize    int64
	Width           int
	Height          int
	Format          string
	CodecID         string
	DurationSeconds float64
	FileNumber      int // Current file number
	TotalFiles      int // Total files to process
	FolderNumber    int // Current folder number
	TotalFolders    int // Total folders
	GPUIndex        int // GPU index for multi-GPU encoding
}

// Stats tracks conversion statistics
type Stats struct {
	Mu             sync.Mutex
	FilesAnalyzed  int
	FilesProcessed int
	FilesImproved  int
	FilesDiscarded int
	FilesSkipped   int
	FilesErrored   int
	OriginalBytes  int64
	FinalBytes     int64
	TouchedDrives  map[string]bool
}

// VideoInfo holds media information about a video file
type VideoInfo struct {
	Format  string
	Width   int
	Height  int
	CodecID string
}
