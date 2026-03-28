//go:build windows

package app

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// getDiskFreeGB returns (availableGB, totalGB) for the given path using the
// Windows GetDiskFreeSpaceExW API.
func getDiskFreeGB(path string) (freeGB, totalGB float64, ok bool) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")
	driveName, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var availBytes, totalBytes, freeBytes uint64
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(driveName)),
		uintptr(unsafe.Pointer(&availBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&freeBytes)),
	)
	if ret == 0 {
		return 0, 0, false
	}
	return float64(availBytes) / (1024 * 1024 * 1024),
		float64(totalBytes) / (1024 * 1024 * 1024),
		true
}

// getAvailableDrives returns a list of available Windows drive roots with
// optional free/total space annotations.
func getAvailableDrives() []string {
	var drives []string
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drivePath := string(drive) + ":\\"
		if _, err := os.Stat(drivePath); err != nil {
			continue
		}
		if freeGB, totalGB, ok := getDiskFreeGB(drivePath); ok {
			drives = append(drives, fmt.Sprintf("%s (%.1f GB free / %.1f GB total)", drivePath, freeGB, totalGB))
		} else {
			drives = append(drives, drivePath)
		}
	}
	return drives
}
