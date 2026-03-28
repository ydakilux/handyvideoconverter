//go:build !windows

package app

import (
	"os/exec"
	"runtime"
)

// openURL opens a URL in the default browser.
// Uses "open" on macOS and "xdg-open" on Linux.
func openURL(url string) error {
	cmd := browserCmd(url)
	return cmd.Start()
}

// openFolder opens a directory in the platform file manager.
func openFolder(path string) error {
	cmd := browserCmd(path)
	return cmd.Start()
}

func browserCmd(target string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", target)
	}
	return exec.Command("xdg-open", target)
}
