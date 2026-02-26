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

type FallbackManager struct {
	interactive bool
	stdin       io.Reader
	logger      *logrus.Logger
	mu          sync.Mutex
}

func NewFallbackManager(interactive bool, stdinReader io.Reader, logger *logrus.Logger) *FallbackManager {
	return &FallbackManager{
		interactive: interactive,
		stdin:       stdinReader,
		logger:      logger,
	}
}

func (fm *FallbackManager) HandleGPUError(stderr string, enc encoder.Encoder, job fmt.Stringer) (bool, error) {
	isGPUError, msg := enc.ParseError(stderr)
	if !isGPUError {
		return false, nil
	}

	if !fm.interactive {
		fm.logger.Warnf("GPU encoding failed for %s: %s — auto-falling back to CPU", job, msg)
		return true, nil
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()


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
