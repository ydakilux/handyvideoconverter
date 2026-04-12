//go:build !windows

package app

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"video-converter/internal/tui"
)

// getDiskFreeBytes returns (availableBytes, totalBytes) for the given path
// using syscall.Statfs (available on Linux and macOS).
func getDiskFreeBytes(path string) (freeBytes, totalBytes uint64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	blockSize := uint64(stat.Bsize)
	return stat.Bavail * blockSize, stat.Blocks * blockSize, true
}

// mountRoots returns candidate mount points to present as "drives" on Unix.
// It checks common prefixes for removable/external media and falls back to
// the filesystem root when nothing else is mounted.
func mountRoots() []string {
	candidates := []string{
		"/Volumes",   // macOS external drives
		"/media",     // Linux (Ubuntu/Debian automount)
		"/mnt",       // Linux manual mounts
		"/run/media", // Linux (Fedora/Arch automount)
	}
	var roots []string
	for _, base := range candidates {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				roots = append(roots, base+"/"+e.Name())
			}
		}
	}
	// Always include filesystem root.
	roots = append(roots, "/")
	return roots
}

// getAvailableDrives returns a list of accessible mount points with
// free/total space annotations and raw free bytes for space checks.
func getAvailableDrives() []tui.DriveInfo {
	var drives []tui.DriveInfo
	for _, root := range mountRoots() {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		freeB, totalB, ok := getDiskFreeBytes(root)
		if ok {
			label := fmt.Sprintf("%s (%.1f GB free / %.1f GB total)",
				root,
				float64(freeB)/(1024*1024*1024),
				float64(totalB)/(1024*1024*1024),
			)
			drives = append(drives, tui.DriveInfo{Root: root, Label: label, FreeBytes: int64(freeB)})
		} else {
			drives = append(drives, tui.DriveInfo{Root: root, Label: root})
		}
	}
	return drives
}
