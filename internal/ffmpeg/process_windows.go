//go:build windows

package ffmpeg

import (
	"os/exec"
	"strconv"
	"syscall"
)

const processAllAccess = 0x1F0FFF

var (
	ntdll             = syscall.NewLazyDLL("ntdll.dll")
	procNtSuspendProc = ntdll.NewProc("NtSuspendProcess")
	procNtResumeProc  = ntdll.NewProc("NtResumeProcess")
)

// suspendProcess suspends all threads of a running process.
func suspendProcess(pid int) {
	handle, err := syscall.OpenProcess(processAllAccess, false, uint32(pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)
	procNtSuspendProc.Call(uintptr(handle)) //nolint:errcheck
}

// resumeProcess resumes a previously suspended process.
func resumeProcess(pid int) {
	handle, err := syscall.OpenProcess(processAllAccess, false, uint32(pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)
	procNtResumeProc.Call(uintptr(handle)) //nolint:errcheck
}

// killProcess force-kills a process and its children using taskkill.
func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.Run() //nolint:errcheck
}
