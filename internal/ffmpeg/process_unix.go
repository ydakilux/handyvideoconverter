//go:build !windows

package ffmpeg

import (
	"os"
	"os/exec"
	"syscall"
)

// suspendProcess suspends a process using SIGSTOP (Unix).
func suspendProcess(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	p.Signal(syscall.SIGSTOP) //nolint:errcheck
}

// resumeProcess resumes a suspended process using SIGCONT (Unix).
func resumeProcess(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	p.Signal(syscall.SIGCONT) //nolint:errcheck
}

// killProcess force-kills a process and its children.
func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Kill the entire process group so child processes are also terminated.
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck
}
