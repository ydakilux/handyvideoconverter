# Makefile for video-converter
# Go-based CLI batch video conversion tool using FFmpeg
#
# Usage: make [target]
# Run 'make help' for available targets.

# ──────────────────────────────────────────────────────────────────
# Variables
# ──────────────────────────────────────────────────────────────────
BINARY_WIN       := video-converter.exe
BINARY_LINUX     := video-converter
BENCHMARK_WIN    := benchmark.exe
BENCHMARK_LINUX  := benchmark
LDFLAGS          := -s -w
COVERAGE         := coverage.out

# Detect if running under WSL or Linux to set default binary name
ifeq ($(OS),Windows_NT)
  BINARY_NAME    := $(BINARY_WIN)
  BENCHMARK_NAME := $(BENCHMARK_WIN)
else
  BINARY_NAME    := $(BINARY_LINUX)
  BENCHMARK_NAME := $(BENCHMARK_LINUX)
endif

# ──────────────────────────────────────────────────────────────────
# Default target
# ──────────────────────────────────────────────────────────────────
.DEFAULT_GOAL := all

.PHONY: all build build-windows build-linux build-wsl build-all release release-windows release-linux release-all \
        benchmark benchmark-windows benchmark-linux benchmark-all \
        test test-verbose test-race test-short test-cover cover cover-html fmt vet lint tidy clean help

all: lint test build  ## Run lint + test + build (auto-detects OS)

# ──────────────────────────────────────────────────────────────────
# Build
# ──────────────────────────────────────────────────────────────────
build:  ## Development build for current OS
	go build -o $(BINARY_NAME)

build-windows:  ## Development build for Windows
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_WIN)

build-linux:  ## Development build for Linux / WSL
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_LINUX)

build-wsl: build-linux  ## Alias: development build for WSL (same as build-linux)

build-all: build-windows build-linux  ## Development build for all platforms

release:  ## Production build for current OS (stripped symbols, smaller binary)
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)

release-windows:  ## Production build for Windows
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_WIN)

release-linux:  ## Production build for Linux / WSL
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_LINUX)

release-all: release-windows release-linux  ## Production build for all platforms

# ──────────────────────────────────────────────────────────────────
# Benchmark tool
# ──────────────────────────────────────────────────────────────────
benchmark:  ## Build benchmark tool for current OS
	go build -ldflags="$(LDFLAGS)" -o $(BENCHMARK_NAME) ./cmd/benchmark

benchmark-windows:  ## Build benchmark tool for Windows
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BENCHMARK_WIN) ./cmd/benchmark

benchmark-linux:  ## Build benchmark tool for Linux / WSL
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BENCHMARK_LINUX) ./cmd/benchmark

benchmark-all: benchmark-windows benchmark-linux  ## Build benchmark tool for all platforms

# ──────────────────────────────────────────────────────────────────
# Test
# ──────────────────────────────────────────────────────────────────
test:  ## Run all tests
	go test ./...

test-verbose:  ## Run tests with verbose output
	go test -v ./...

test-race:  ## Run tests with race detector
	go test -race ./...

test-short:  ## Run only fast unit tests (skip integration tests that need ffmpeg/GPU)
	go test -short ./...

test-cover:  ## Run tests and print per-package coverage summary
	go test -cover ./...

cover:  ## Generate coverage report and open in browser
	go test -coverprofile=$(COVERAGE) ./...
	go tool cover -html=$(COVERAGE)

cover-html: cover  ## Alias for cover (generate HTML report)

# ──────────────────────────────────────────────────────────────────
# Code quality
# ──────────────────────────────────────────────────────────────────
fmt:  ## Format code
	go fmt ./...

vet:  ## Run static analysis
	go vet ./...

lint: fmt vet  ## Run fmt + vet

# ──────────────────────────────────────────────────────────────────
# Dependencies
# ──────────────────────────────────────────────────────────────────
tidy:  ## Tidy and verify module dependencies
	go mod tidy
	go mod verify

# ──────────────────────────────────────────────────────────────────
# Cleanup
# ──────────────────────────────────────────────────────────────────
clean:  ## Remove build artifacts and coverage files
	go clean
	-del /Q $(BINARY_WIN) 2>nul
	-rm -f $(BINARY_LINUX)
	-del /Q $(BENCHMARK_WIN) 2>nul
	-rm -f $(BENCHMARK_LINUX)
	-del /Q $(COVERAGE) 2>nul
	-rm -f $(COVERAGE)

# ──────────────────────────────────────────────────────────────────
# Help
# ──────────────────────────────────────────────────────────────────
help:  ## Print available targets with descriptions
	@echo.
	@echo Available targets:
	@echo.
	@echo   make all              Run lint + test + build for current OS (default)
	@echo   make build            Development build for current OS
	@echo   make build-windows    Development build for Windows (.exe)
	@echo   make build-linux      Development build for Linux
	@echo   make build-wsl        Alias for build-linux (WSL)
	@echo   make build-all        Development build for all platforms
	@echo   make release          Production build for current OS (stripped symbols)
	@echo   make release-windows  Production build for Windows
	@echo   make release-linux    Production build for Linux / WSL
	@echo   make release-all      Production build for all platforms
	@echo   make benchmark        Build benchmark tool for current OS
	@echo   make benchmark-windows Build benchmark tool for Windows
	@echo   make benchmark-linux  Build benchmark tool for Linux / WSL
	@echo   make benchmark-all    Build benchmark tool for all platforms
	@echo   make test             Run all tests
	@echo   make test-verbose     Run tests with verbose output
	@echo   make test-race        Run tests with race detector
	@echo   make test-short       Run only fast unit tests (no ffmpeg/GPU required)
	@echo   make test-cover       Run tests and print per-package coverage summary
	@echo   make cover            Generate coverage report (opens browser)
	@echo   make cover-html       Alias for cover
	@echo   make fmt              Format code
	@echo   make vet              Run static analysis
	@echo   make lint             Run fmt + vet
	@echo   make tidy             Tidy and verify module dependencies
	@echo   make clean            Remove build artifacts and coverage files
	@echo   make help             Print this help message
	@echo.
