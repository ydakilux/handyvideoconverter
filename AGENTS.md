# Agent Guidelines for Video Converter

This document provides coding standards and development commands for AI agents working on this Go-based FFmpeg video conversion tool.

## Project Overview

A CLI batch video conversion tool for Windows that converts videos to MP4 (HEVC/H.265) using FFmpeg, with per-drive caching, producer-consumer pipeline, optional Seq logging, and detailed per-file conversion reporting with size comparisons.

## Build & Run Commands

### Setup
```bash
# Download dependencies
go mod download

# Tidy dependencies and create/update go.sum
go mod tidy

# Verify module integrity
go mod verify
```

### Build
```bash
# Development build
go build -o video-converter.exe

# Production build (optimized, smaller binary)
go build -ldflags="-s -w" -o video-converter.exe

# Cross-compile (if needed)
GOOS=windows GOARCH=amd64 go build -o video-converter.exe
```

### Run
```bash
# Basic usage
./video-converter.exe D:\Videos\

# With flags
./video-converter.exe --config myconfig.json --dry-run --encoder libx265 D:\Media\

# Interactive mode (prompts for bypass/force-hevc)
./video-converter.exe D:\Videos\

# Show help
./video-converter.exe --help
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a single test function
go test -v -run TestFunctionName

# Run specific test in a package
go test -v -run TestGetFileHash ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run tests with race detector
go test -race ./...

# Benchmark tests
go test -bench=. ./...

# Run tests with timeout
go test -timeout 30s ./...

# Makefile shortcuts
make test           # go test ./... -count=1 -timeout 120s
make test-short     # go test -short ./... (skips integration tests)
make test-cover     # run tests and print per-package coverage
make cover-html     # open coverage report in browser
```

### Linting & Formatting
```bash
# Format code (required before commits)
go fmt ./...

# Run go vet for static analysis
go vet ./...

# Install and run golangci-lint (recommended)
golangci-lint run

# Check for suspicious constructs
go vet ./...

# Use staticcheck if installed
staticcheck ./...
```

## Code Style Guidelines

### Import Organization
Imports MUST be organized in three groups separated by blank lines:
1. Standard library packages (alphabetical)
2. Third-party packages (alphabetical)
3. Local/internal packages

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)
```

### Naming Conventions
- **Variables**: camelCase (`fileHash`, `driveRoot`, `totalDuration`)
- **Constants**: PascalCase or UPPER_SNAKE_CASE (`MaxQueueSize`, `DEFAULT_CHUNK_SIZE`)
- **Functions**: camelCase for private, PascalCase for exported (`getFileHash`, `ProcessConversion`)
- **Structs**: PascalCase (`Config`, `DatabaseManager`, `VideoInfo`)
- **Interfaces**: PascalCase ending in -er when possible (`Reader`, `Converter`, `Hook`)
- **Acronyms**: Maintain case consistency (URL, ID, JSON, HTTP not Url, Id, Json, Http)

### Types & Struct Tags
- Use struct tags for JSON marshaling: `json:"field_name,omitempty"`
- Define custom types for clarity: `type Record struct { ... }`
- Use pointers for large structs passed to functions
- Embed interfaces when appropriate
- Document exported types with comments starting with the type name

### Error Handling
- ALWAYS check errors immediately after function calls
- Use `fmt.Errorf` with `%w` to wrap errors for context
- Log errors before returning them: `log.Errorf("Failed to X: %v", err)`
- Return errors as last return value: `func foo() (string, error)`
- Use early returns for error conditions
- Never ignore errors with `_` unless explicitly documented why

```go
// Good
info, err := os.Stat(filePath)
if err != nil {
	log.Errorf("Failed to stat file %s: %v", filePath, err)
	return "", fmt.Errorf("stat failed: %w", err)
}

// Bad - missing error check
info, _ := os.Stat(filePath)
```

### Concurrency Patterns
- Use sync.Mutex for protecting shared state (DatabaseManager)
- Prefer channels for goroutine communication
- Close channels when done producing: `defer close(jobQueue)`
- Use sync.WaitGroup for coordinating goroutine completion
- Document thread-safety requirements in comments
- Avoid goroutine leaks - ensure all goroutines can exit

### Logging Standards
- Use logrus logger from global `log` variable
- Log levels: Debug, Info, Warn, Error, Fatal
- Include context in log messages: file paths, sizes, percentages
- Format: `log.Infof("SUCCESS: %s -> %s (%.1f%% reduction)", src, dst, pct)`
- Use structured logging with fields when appropriate

### Function Design
- Keep functions focused on a single responsibility
- Limit function length to ~50 lines when possible
- Use descriptive parameter names: `func getFileHash(filePath string, partial bool)`
- Document exported functions with comments starting with function name
- Return early for error conditions
- Avoid deeply nested conditionals

### Context & Timeouts
- Use context.WithTimeout for external command execution
- Default timeout for MediaInfo/FFprobe: 30 seconds
- Always defer cancel(): `defer cancel()`
- Pass context to long-running operations

### File Operations (Windows-specific)
- Use filepath package for path manipulation (not string concat)
- Always use filepath.Join() for constructing paths
- Handle Windows paths correctly: `filepath.VolumeName()`, backslashes
- Create directories with os.MkdirAll before writing files
- Atomic file writes: write to .tmp then rename with os.Rename
- Use forward slashes (/) in go.mod and import paths

### Configuration
- Load config from JSON with defaults
- Resolve relative paths relative to executable directory using filepath
- Validate configuration after loading
- Use struct tags for JSON field mapping
- Create default config with proper indentation if missing

### Comments
- Comment exported functions, types, and constants
- Use TODO comments for future improvements: `// TODO: implement retry logic`
- Explain WHY, not WHAT, for complex logic
- Keep comments concise and up-to-date
- Use godoc format for package documentation

### Command Execution
- Use exec.CommandContext for timeouts
- Stream stdout/stderr when processing progress
- Check exit codes explicitly
- Handle process cleanup properly (defer close pipes)

## Project-Specific Conventions

### Hash Algorithm
- Use BLAKE3 for file hashing (github.com/zeebo/blake3)
- Partial hash: 16MB start + 16MB middle + 16MB end + file size
- Full hash: stream 128KB chunks
- Hash is hex-encoded string

### Database Management
- One JSON cache per drive root: `D:\converted_files.json`
- Thread-safe access via DatabaseManager with RWMutex
- Atomic writes using .tmp + rename pattern
- Flush after each job completion for safety
- Key is file hash, value is Record struct

### FFmpeg Integration
- Parse progress via stdout: `out_time_ms`, `out_time_us`, `out_time`
- Report progress every 5% increment
- Use flags: `-hide_banner`, `-y`, `-nostats`, `-progress pipe:1`
- Container format: always MP4 with `-movflags +faststart`
- Quality parameter: use `-crf` for libx265, `-cq` for nvenc encoders

### Output Organization
- Final location: `<Drive>\HSORTED\<ParentFolder>\<filename>.mp4`
- Temp location: `<Drive>\HSORTED\_TEMP\<ParentFolder>\__tmp__<hash8>.mp4`
- Sanitize folder names: replace `[\/:*?"<>|]` with `_`
- Handle duplicate filenames by appending hash

### Seq Logging
- Custom SeqHook implementation (not external package)
- Sends JSON events to Seq server via HTTP POST
- Uses X-Seq-ApiKey header when API key provided
- Gracefully continues if Seq is unavailable

## Testing Requirements

- Test file discovery with various extensions (case-insensitive)
- Test hash consistency (same file = same hash)
- Mock external executables (MediaInfo, FFmpeg, FFprobe)
- Test database load/save atomicity
- Test concurrent producer/consumer pipeline
- Validate output path construction on Windows
- Test drive root detection with filepath.VolumeName

## Common Pitfalls to Avoid

1. Don't use string concatenation for paths (use filepath.Join)
2. Don't ignore errors from os.Stat, os.Remove, os.Rename
3. Don't forget to close channels after producer finishes
4. Don't block on channel sends without proper buffering
5. Don't use defer in long-running loops (resource leaks)
6. Don't hardcode backslashes in go.mod (use forward slashes)
7. Don't forget context cancellation (defer cancel())
8. Don't use absolute paths in go.mod - package paths use forward slashes
9. Don't forget to run `go mod tidy` after changing dependencies
10. Don't assume paths - resolve executables via PATH if not found

## Windows-Specific Notes

- Drive roots use backslashes: `D:\`
- Use `filepath.VolumeName()` to extract drive letter
- Use `cmd /c start "" "<folder>"` to open folders in Explorer
- Executable names include .exe extension
- Config paths with backslashes must be escaped in JSON: `"ffmpeg\\bin\\ffmpeg.exe"`
- **FFmpeg resolution**: `exec.LookPath("ffmpeg")` resolves to the real `ffmpeg.exe` on PATH (e.g. `C:\ProgramData\chocolatey\bin\ffmpeg.exe`). The legacy `ffmpeg.cmd` WSL wrapper has been removed. Integration tests use a `lookupExe` helper that prefers `.exe` over `.cmd` when both are on PATH.
