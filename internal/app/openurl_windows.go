//go:build windows

package app

import "os/exec"

// openURL opens a URL in the default browser using the Windows shell.
func openURL(url string) error {
	return exec.Command("cmd", "/c", "start", "", url).Start()
}

// openFolder opens a folder in Windows Explorer.
func openFolder(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
