//go:build windows

package app

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/ydakilux/reforge/internal/tui"
)

// getDiskFreeBytes returns (availableBytes, totalBytes) for the given path using
// the Windows GetDiskFreeSpaceExW API.
func getDiskFreeBytes(path string) (freeBytes, totalBytes uint64, ok bool) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")
	driveName, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var avail, total, free uint64
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(driveName)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if ret == 0 {
		return 0, 0, false
	}
	return avail, total, true
}

// getAvailableDrives returns a list of available Windows drive roots with
// free/total space annotations and raw free bytes for space checks.
func getAvailableDrives() []tui.DriveInfo {
	var drives []tui.DriveInfo
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drivePath := string(drive) + ":\\"
		if _, err := os.Stat(drivePath); err != nil {
			continue
		}
		freeB, totalB, ok := getDiskFreeBytes(drivePath)
		if ok {
			label := fmt.Sprintf("%s (%.1f GB free / %.1f GB total)",
				drivePath,
				float64(freeB)/(1024*1024*1024),
				float64(totalB)/(1024*1024*1024),
			)
			drives = append(drives, tui.DriveInfo{Root: drivePath, Label: label, FreeBytes: int64(freeB)})
		} else {
			drives = append(drives, tui.DriveInfo{Root: drivePath, Label: drivePath})
		}
	}
	return drives
}
