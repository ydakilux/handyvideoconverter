// Package fallback provides GPU-to-CPU fallback logic when a hardware encoder
// fails mid-conversion.
package fallback

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"

	"video-converter/internal/encoder"

	"github.com/sirupsen/logrus"
)

// FallbackManager detects GPU encoding failures and optionally prompts the user to retry with CPU.
type FallbackManager struct {
	interactive bool
	stdin       io.Reader
	logger      *logrus.Logger
	// promptMu serializes interactive GPU-error prompts so that concurrent
	// worker goroutines don't interleave their output to stdin/stdout.
	// It is intentionally held while blocking on stdin.Scan() because we want
	// exactly one prompt at a time; subsequent goroutines queue up and receive
	// the mutex only after the current user has answered.
	promptMu sync.Mutex
}

// NewFallbackManager creates a FallbackManager. When interactive is true, GPU errors prompt the user via stdinReader; otherwise fallback is automatic.
func NewFallbackManager(interactive bool, stdinReader io.Reader, logger *logrus.Logger) *FallbackManager {
	return &FallbackManager{
		interactive: interactive,
		stdin:       stdinReader,
		logger:      logger,
	}
}

// HandleGPUError checks whether stderr indicates a GPU-specific failure and, if so, either prompts the user or silently falls back to CPU.
func (fm *FallbackManager) HandleGPUError(stderr string, enc encoder.Encoder, job fmt.Stringer) (bool, error) {
	isGPUError, msg := enc.ParseError(stderr)
	if !isGPUError {
		return false, nil
	}

	if !fm.interactive {
		fm.logger.Warnf("GPU encoding failed for %s: %s — auto-falling back to CPU", job, msg)
		return true, nil
	}

	fm.promptMu.Lock()
	defer fm.promptMu.Unlock()

	prompt := fmt.Sprintf("GPU encoding failed for %s: %s\nRetry with CPU? [Y/n] ", job, msg)
	fmt.Fprint(logWriter{fm.logger}, prompt)

	scanner := bufio.NewScanner(fm.stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading stdin: %w", err)
		}
		fm.logger.Warn("EOF on stdin, defaulting to CPU fallback")
		return true, nil
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer == "" || answer == "y" || answer == "yes" {
		return true, nil
	}
	return false, nil
}

type logWriter struct {
	logger *logrus.Logger
}

func (w logWriter) Write(p []byte) (int, error) {
	w.logger.Info(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
