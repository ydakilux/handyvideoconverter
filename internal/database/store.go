package database

import (
	"context"

	"video-converter/internal/types"
)

// Stats holds aggregate conversion statistics.
type Stats struct {
	TotalFiles     int
	TotalOriginal  int64
	TotalConverted int64
	ErrorCount     int
	NotBeneficial  int
	AlreadyHEVC    int
	SuccessCount   int
}

// ErrorRecord represents a failed conversion.
type ErrorRecord struct {
	FileHash     string
	DriveRoot    string
	SourcePath   string
	OriginalSize int64
	Error        string
	ConvertedAt  string
}

// NotBeneficialRecord represents a conversion where the output was larger.
type NotBeneficialRecord struct {
	FileHash      string
	DriveRoot     string
	SourcePath    string
	OriginalSize  int64
	ConvertedSize int64
}

// RecentRecord represents a recent conversion with full details.
type RecentRecord struct {
	FileHash      string
	DriveRoot     string
	SourcePath    string
	OriginalSize  int64
	ConvertedSize int64
	OutputPath    string
	Note          string
	Error         string
	SourceCodec   string
	ConvertedAt   string
}

// FormatStat represents conversion stats for a source format.
type FormatStat struct {
	SourceCodec     string
	SourceContainer string
	Count           int
	TotalOriginal   int64
	TotalConverted  int64
}

// SpaceSavedResult holds space saved aggregation.
type SpaceSavedResult struct {
	Period     string
	FileCount  int
	BytesSaved int64
}

// Store defines the interface for conversion record storage.
type Store interface {
	// GetRecord retrieves a conversion record by drive root and file hash.
	// Returns (nil, nil) if the record does not exist.
	GetRecord(ctx context.Context, driveRoot, fileHash string) (*types.Record, error)

	// UpdateRecord creates or updates a conversion record.
	UpdateRecord(ctx context.Context, driveRoot, fileHash string, rec types.Record) error

	// Close closes the store and releases resources.
	Close() error

	// GetStats returns aggregate statistics, optionally filtered by drive root.
	// Pass driveRoot="" for all drives.
	GetStats(ctx context.Context, driveRoot string) (*Stats, error)

	// GetErrors returns records that failed conversion.
	GetErrors(ctx context.Context, driveRoot, pathFilter string) ([]ErrorRecord, error)

	// GetNotBeneficial returns records where conversion was not beneficial.
	GetNotBeneficial(ctx context.Context, driveRoot string) ([]NotBeneficialRecord, error)

	// GetRecent returns the most recent conversion records.
	GetRecent(ctx context.Context, limit int) ([]RecentRecord, error)

	// GetFormatBreakdown returns conversion statistics grouped by source format.
	GetFormatBreakdown(ctx context.Context, driveRoot string) ([]FormatStat, error)

	// GetSpaceSaved returns total space saved for a given time period.
	// period: "week" (7 days), "month" (30 days), "total" (all time)
	GetSpaceSaved(ctx context.Context, period string) (*SpaceSavedResult, error)
}
