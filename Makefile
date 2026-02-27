# Makefile for video-converter
# Go-based CLI batch video conversion tool using FFmpeg
#
# Usage: make [target]
# Run 'make help' for available targets.

# ──────────────────────────────────────────────────────────────────
# Variables
# ──────────────────────────────────────────────────────────────────
BINARY_NAME := video-converter.exe
LDFLAGS     := -s -w
COVERAGE    := coverage.out

# ──────────────────────────────────────────────────────────────────
# Default target
# ──────────────────────────────────────────────────────────────────
.DEFAULT_GOAL := all

.PHONY: all build release test test-verbose test-race cover fmt vet lint tidy clean help

all: lint test build  ## Run lint + test + build

# ──────────────────────────────────────────────────────────────────
# Build
# ──────────────────────────────────────────────────────────────────
build:  ## Development build
	go build -o $(BINARY_NAME)

release:  ## Production build (stripped symbols, smaller binary)
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)

# ──────────────────────────────────────────────────────────────────
# Test
# ──────────────────────────────────────────────────────────────────
test:  ## Run all tests
	go test ./...

test-verbose:  ## Run tests with verbose output
	go test -v ./...

test-race:  ## Run tests with race detector
	go test -race ./...

cover:  ## Generate coverage report and open in browser
	go test -coverprofile=$(COVERAGE) ./...
	go tool cover -html=$(COVERAGE)

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
	-del /Q $(BINARY_NAME) 2>nul
	-del /Q $(COVERAGE) 2>nul

# ──────────────────────────────────────────────────────────────────
# Help
# ──────────────────────────────────────────────────────────────────
help:  ## Print available targets with descriptions
	@echo.
	@echo Available targets:
	@echo.
	@echo   make all            Run lint + test + build (default)
	@echo   make build          Development build
	@echo   make release        Production build (stripped symbols)
	@echo   make test           Run all tests
	@echo   make test-verbose   Run tests with verbose output
	@echo   make test-race      Run tests with race detector
	@echo   make cover          Generate coverage report (opens browser)
	@echo   make fmt            Format code
	@echo   make vet            Run static analysis
	@echo   make lint           Run fmt + vet
	@echo   make tidy           Tidy and verify module dependencies
	@echo   make clean          Remove build artifacts and coverage files
	@echo   make help           Print this help message
	@echo.
