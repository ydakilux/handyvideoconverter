// Package fileutil provides shared file path helpers, string formatting
// utilities, and the BLAKE3-based file hashing used across the converter pipeline.
package fileutil

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

const HashChunkSize = 16 * 1024 * 1024 // 16 MB

// GetDriveRoot returns the volume root for filePath.
// On Windows this is the drive root (e.g. "D:\").
// On Unix it returns "/" for absolute paths; for paths with no volume it also
// returns "/".
func GetDriveRoot(filePath string) string {
	vol := filepath.VolumeName(filePath)
	if vol == "" {
		return "/"
	}
	return vol + string(filepath.Separator)
}

// GetParentFolderName returns the immediate parent folder name of filePath
// relative to driveRoot. Returns "ROOT" when the file sits directly on the drive.
func GetParentFolderName(filePath, driveRoot string) string {
	dir := filepath.Dir(filePath)
	cleanRoot := filepath.Clean(driveRoot)
	if filepath.Clean(dir) == cleanRoot {
		return "ROOT"
	}
	return filepath.Base(dir)
}

// GetRelativePath returns the path from driveRoot to the file's directory.
// Returns "ROOT" when the file sits directly on the drive.
// Currently only exercised by tests; kept for potential future use.
func GetRelativePath(filePath, driveRoot string) string {
	dir := filepath.Dir(filePath)

	driveRoot = filepath.Clean(driveRoot)
	dir = filepath.Clean(dir)

	if dir == driveRoot {
		return "ROOT"
	}

	relPath, err := filepath.Rel(driveRoot, dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return filepath.Clean(relPath)
}

// SanitizeFolderName replaces Windows-invalid characters in each path segment
// with underscores while preserving path separators.
func SanitizeFolderName(name string) string {
	parts := strings.Split(name, string(filepath.Separator))
	invalid := []string{":", "*", "?", "\"", "<", ">", "|"}
	for i, part := range parts {
		result := part
		for _, char := range invalid {
			result = strings.ReplaceAll(result, char, "_")
		}
		parts[i] = result
	}
	return filepath.Join(parts...)
}

// GetFileHash returns a hex-encoded BLAKE3 hash for filePath.
// When partial is true, only the first 16 MB, middle 16 MB, last 16 MB, and
// the file size are hashed — fast and sufficient for large media files.
func GetFileHash(filePath string, partial bool) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}
	fileSize := info.Size()

	hasher := blake3.New()

	if partial {
		buf := make([]byte, HashChunkSize)
		n, _ := io.ReadFull(f, buf)
		hasher.Write(buf[:n])

		if fileSize > HashChunkSize*2 {
			middle := fileSize / 2
			f.Seek(middle, 0) //nolint:errcheck
			n, _ = io.ReadFull(f, buf)
			hasher.Write(buf[:n])
		}

		if fileSize > HashChunkSize {
			f.Seek(-HashChunkSize, 2) //nolint:errcheck
			n, _ = io.ReadFull(f, buf)
			hasher.Write(buf[:n])
		}

		sizeBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(sizeBuf, uint64(fileSize))
		hasher.Write(sizeBuf)
	} else {
		buf := make([]byte, 128*1024)
		if _, err := io.CopyBuffer(hasher, f, buf); err != nil {
			return "", fmt.Errorf("copy: %w", err)
		}
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// FormatBytes returns a human-readable size string (e.g. "1.5 MB").
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FmtElapsed formats a duration as "Xh MMm SSs", "Mm SSs", or "Xs".
func FmtElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// TruncateString shortens s to maxLen characters, appending "..." if truncated.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
