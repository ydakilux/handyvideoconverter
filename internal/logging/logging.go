package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// SimpleFormatter outputs only the log message without level or other metadata
type SimpleFormatter struct{}

func (f *SimpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + "\n"), nil
}

// SeqHook sends logs to Seq
type SeqHook struct {
	serverURL string
	apiKey    string
	client    *http.Client
}

// NewSeqHook creates a new Seq hook
func NewSeqHook(serverURL, apiKey string) *SeqHook {
	return &SeqHook{
		serverURL: strings.TrimSuffix(serverURL, "/"),
		apiKey:    apiKey,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Levels returns the log levels this hook should fire on
func (hook *SeqHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire sends the log entry to Seq
func (hook *SeqHook) Fire(entry *logrus.Entry) error {
	// Build Seq event
	event := map[string]interface{}{
		"@t":  entry.Time.Format(time.RFC3339Nano),
		"@mt": entry.Message,
		"@l":  entry.Level.String(),
	}

	// Add fields
	for k, v := range entry.Data {
		event[k] = v
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Send to Seq
	req, err := http.NewRequest("POST", hook.serverURL+"/api/events/raw", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if hook.apiKey != "" {
		req.Header.Set("X-Seq-ApiKey", hook.apiKey)
	}

	resp, err := hook.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// SetupLogging creates and configures a logrus.Logger with file output,
// SimpleFormatter, optional Seq hook, and the specified log level.
// It returns the logger and a cleanup function that closes the log file.
	func SetupLogging(serverURL, apiKey, logLevel, execDir string) (*logrus.Logger, func()) {
	logger := logrus.New()
	cleanup := func() {} // no-op default
	// Create log file with timestamp
	logFileName := fmt.Sprintf("video-converter_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logFilePath := filepath.Join(execDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
	} else {
		// Write to both console and file
		mw := io.MultiWriter(os.Stdout, logFile)
		logger.SetOutput(mw)
		fmt.Printf("Logging to: %s\n", logFilePath)
		cleanup = func() {
			logger.SetOutput(io.Discard)
			logFile.Close()
		}
	}
	// Custom formatter that only outputs the message (for console)
	logger.SetFormatter(&SimpleFormatter{})
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	if serverURL != "" {
		hook := NewSeqHook(serverURL, apiKey)
		logger.AddHook(hook)
		logger.Debug("Seq logging hook enabled")
	}

	return logger, cleanup
}
