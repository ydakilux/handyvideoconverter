//go:build !windows

package app

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// getDiskFreeGB returns (availableGB, totalGB) for the given path using
// syscall.Statfs (available on Linux and macOS).
func getDiskFreeGB(path string) (freeGB, totalGB float64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, false
	}
	blockSize := uint64(stat.Bsize)
	total := float64(stat.Blocks*blockSize) / (1024 * 1024 * 1024)
	free := float64(stat.Bavail*blockSize) / (1024 * 1024 * 1024)
	return free, total, true
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

// getAvailableDrives returns a list of accessible mount points with optional
// free/total space annotations. On Unix there are no "drive letters"; instead
// we surface common mount points (external drives, /mnt, /Volumes …) plus /.
func getAvailableDrives() []string {
	var drives []string
	for _, root := range mountRoots() {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		if freeGB, totalGB, ok := getDiskFreeGB(root); ok {
			drives = append(drives, fmt.Sprintf("%s (%.1f GB free / %.1f GB total)", root, freeGB, totalGB))
		} else {
			drives = append(drives, root)
		}
	}
	return drives
}
