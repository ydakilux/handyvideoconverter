# Architectural Review — HandyVideoConverter

> Reviewed: 2026-03-21 | Scope: all 42 source files | Reviewer: AI Architect

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Architecture Overview](#architecture-overview)
3. [Strengths](#strengths)
4. [Critical Issues](#critical-issues)
5. [Refactoring Recommendations](#refactoring-recommendations)
6. [Scalability & Technical Debt](#scalability--technical-debt)
7. [Priority Action Plan](#priority-action-plan)

---

## Executive Summary

The codebase has a **solid internal package structure** with well-applied patterns (Strategy, Registry, Producer-Consumer, Controller). However, `main.go` has become a 1,200+ line God Object that absorbs responsibilities belonging in dedicated packages. Most of the identified issues are **consequences of this single root cause**. Resolving it in phases would eliminate the majority of the issues below without touching the internal packages at all.

---

## Architecture Overview

```
main.go (1,215 lines)              ← God Object: CLI, orchestration, hashing, path
                                     logic, FFmpeg arg builder, stats, drive utils
cmd/benchmark/main.go              ← clean standalone sub-command
internal/
  config/config.go                 ← config load/save, encoder validation
  database/database.go             ← per-drive JSON cache, atomic writes
  encoder/
    encoder.go                     ← Encoder interface (Strategy)
    registry.go                    ← Registry (RWMutex, thread-safe)
    nvenc.go / amf.go / qsv.go / libx265.go
  fallback/fallback.go             ← GPU error recovery
  ffmpeg/ffmpeg.go                 ← FFmpeg process runner, progress parsing
  gpu/
    benchmark/benchmark.go         ← FPS benchmarking, cache in config JSON
    detect/detect.go               ← trial-encode GPU detection
    nvidia/nvidia.go               ← nvidia-smi queries (mostly unused)
  logging/logging.go               ← logrus setup, SeqHook
  pipeline/
    pipeline.go                    ← producer-consumer, context cancellation
    control.go                     ← pause/resume/stop (sync.Cond)
    distributor.go                 ← proportional GPU job distribution
  tui/tui.go                       ← Bubble Tea v1 TUI
  types/types.go                   ← shared data types
```

---

## Strengths

These are well-designed and should be preserved as-is.

| Component | Why it's good |
|---|---|
| `Encoder` interface + Registry | Clean Strategy pattern; adding a new encoder requires only one new file |
| `Controller` (control.go) | `sync.Cond`-based pause/resume is correct and race-free |
| `Distributor` (distributor.go) | Largest-remainder proportional allocation is mathematically correct and tested |
| `DatabaseManager` | Uses `sync.Mutex` (not `RWMutex`) — correct choice, avoids lock-promotion races on read-modify-write paths |
| Atomic file writes | Both database and benchmark cache use tmp-then-rename — safe on power loss |
| `Pipeline` | Encoder-agnostic; uses dependency injection via handler func |
| TUI fallback | Plain-mode fallback for non-TTY environments is clean |

---

## Critical Issues

### 1. `main.go` is a God Object

**Severity: High**

`main.go` (1,215 lines) contains: CLI parsing, config loading, GPU detection, benchmark orchestration, interactive prompts, file discovery, pipeline wiring, conversion logic, file hashing, path manipulation, stats tracking, drive enumeration, and output folder opening. `processConversion()` alone is ~200 lines.

**Impact:** Untestable core logic, merge conflicts, inability to reuse components.

**Fix:** Extract into at minimum `internal/converter/`, `internal/discovery/`, and `internal/app/` packages. See [Refactoring Recommendations](#refactoring-recommendations).

---

### 2. Package-level mutable globals

**Severity: High**

`main.go` declares 12+ package-level variables (`config`, `dbManager`, `stats`, `execDir`, `selectedEncoder`, `ui`, `pipelineCtrl`, etc.) that are mutated during `main()`. These make unit testing nearly impossible and create implicit shared state between functions.

**Fix:** Collect into an `App` struct passed by pointer. Functions become methods; state becomes explicit.

```go
type App struct {
    Config          *types.Config
    DB              *database.Manager
    Stats           *Stats
    UI              tui.Interface
    EncoderRegistry *encoder.Registry
    // ...
}
```

---

### 3. `config.DetermineQuality()` is dead code + duplicated logic

**Severity: Medium**

`internal/config/config.go` exports `DetermineQuality()` but `main.go` never calls it. Each of the four encoder files (`nvenc.go`, `amf.go`, `qsv.go`, `libx265.go`) re-implements an identical resolution-to-CRF/CQ mapping table. The config function is unreachable.

**Fix:** Delete `config.DetermineQuality()`. Define a single `internal/encoder/quality.go` with the canonical resolution→quality table used by all encoders via a shared helper.

---

### 4. `ffmpeg.BuildArgs()` is dead code

**Severity: Medium**

`ffmpeg.go` exports `BuildArgs()` but `main.go` defines its own `buildConversionArgs()` and uses that instead. Two implementations of the same function exist simultaneously.

**Fix:** Delete whichever is less complete. The surviving implementation should live in `internal/ffmpeg/` or a new `internal/converter/` package, not in `main.go`.

---

### 5. Benchmark cache stored inside user config file (TOCTOU risk)

**Severity: Medium**

`benchmark.SaveCache()` reads the entire `configVideoConversion.json`, injects a `benchmark_cache` key, then rewrites the whole file. If the user edits the config simultaneously (or if config is saved by another code path at the same time), data is silently overwritten.

**Fix:** Store benchmark cache in a separate file (e.g., `benchmark_cache.json` alongside the config). Remove the read-modify-write pattern entirely.

---

### 6. `FallbackManager` holds mutex while reading `os.Stdin`

**Severity: Medium**

`fallback.HandleGPUError()` acquires `fm.mu` and then blocks on terminal input (`os.Stdin`). If multiple workers fail simultaneously (possible in parallel mode), all workers queue behind the mutex while the UI is blocked on keyboard input.

**Fix:** Use a separate "one interactive prompt at a time" channel or `sync.Once`-style gate that does not hold `fm.mu` during the I/O wait. Release the mutex before prompting, re-acquire only to update shared state.

---

### 7. `database.SaveAll()` called after every job

**Severity: Medium**

`main.go` calls `dbManager.SaveAll()` inside the per-job pipeline handler. With hundreds of small files this is O(n) full JSON serialization+write operations. The `dirty` flag exists on `DatabaseManager` but is not being leveraged to skip saves.

**Fix:** Call `SaveAll()` only when the dirty flag is set, and flush at pipeline completion + on SIGINT/SIGTERM. This reduces disk writes by orders of magnitude on large batches.

---

### 8. `logging.SetupLogging` called twice, leaking first log file handle

**Severity: Medium**

`main()` calls `SetupLogging` once before TUI init and again after config is loaded. Each call opens a new timestamped log file. The second call shadows the `logCleanup` variable, making the first file's handle uncleanable (never closed).

**Fix:** Call `SetupLogging` exactly once, after config is loaded. If early logging is needed before config, use a temporary in-memory buffer and flush to the real log file after setup.

---

### 9. No context propagation to `processConversion`

**Severity: Medium**

The pipeline passes `ctx` to the job handler, but `processConversion` ignores it. FFmpeg is launched without the pipeline context, so `StopNow` cannot cancel in-flight FFmpeg processes via Go context — it relies entirely on `NtSuspendProcess`/`SuspendFn`. This makes graceful shutdown Windows-only and non-compositional.

**Fix:** Pass `ctx` into `processConversion` and into `ffmpeg.Run()`. Use `exec.CommandContext(ctx, ...)` as the primary cancellation mechanism; keep `NtSuspendProcess` only for pause/resume.

---

### 10. `tui.New()` uses `time.Sleep(50ms)` as a startup synchronization barrier

**Severity: Low**

`tui.New()` sleeps 50ms after starting the Bubble Tea goroutine to "give bubbletea a moment to start." This is a timing hack — on a slow or loaded system it can still race; on a fast system it wastes 50ms.

**Fix:** Use a `chan struct{}` ready signal. The Bubble Tea `Init` command sends on the channel; `New()` waits on receive.

---

### 11. `SeqHook` has no timeout cap or circuit breaker

**Severity: Low**

`SeqHook.Fire()` makes an unbounded HTTP POST on every log event. A misconfigured or unreachable Seq URL causes a multi-second timeout on every log line, severely degrading performance. The error is silently swallowed (correct for resilience) but there is no backoff or disabling after repeated failures.

**Fix:** Add a `consecutiveFailures` counter. After N failures, mark the hook disabled and stop attempting. Log a single warning when this happens.

---

### 12. `Stats` mutex accessed at 15+ call sites in `main.go`

**Severity: Low**

`stats.Mu.Lock()` / `stats.Mu.Unlock()` are called inline at 15+ locations in `main.go`. Any future change to the stats struct risks forgetting a lock site.

**Fix:** Encapsulate stats mutations behind methods on a `Stats` struct: `stats.IncrementProcessed()`, `stats.AddBytes(n)`, etc. Zero external locking required.

---

### 13. `nvidia.QueryGPUs` / `QuerySessionCount` are unreachable from pipeline

**Severity: Low**

`internal/gpu/nvidia/nvidia.go` defines VRAM and session-count queries via `nvidia-smi`, but the main pipeline never calls them. The `Distributor` uses only benchmark FPS data for allocation decisions. Live VRAM data would enable smarter routing (e.g., skip a GPU that is memory-exhausted).

**Note:** Not a bug — but the package is currently dead weight. Either wire it in or mark it with a `// TODO` explaining intended use.

---

### 14. `Config` persists `non_interactive` flag to disk

**Severity: Low**

`types.Config.NonInteractive` is tagged `json:"non_interactive,omitempty"`. If a user runs with `--non-interactive`, the flag is written into `configVideoConversion.json` and silently persists across runs. Most users would expect CLI flags to be session-only.

**Fix:** Tag it `json:"-"` like `Rebenchmark` is, or document this behavior explicitly.

---

### 15. `getAvailableDrives()` variable shadowing — `freeBytes` vs `availBytes`

**Severity: Low**

The function declares `freeBytes` and `availBytes` as separate `uint64` variables and passes both to `GetDiskFreeSpaceExW`. The displayed value uses `freeBytes` (total free) when user intent is available-to-user bytes (`availBytes`). They differ on drives with disk quotas.

**Fix:** Display `availBytes` for accuracy, or combine into a single variable if both are identical in practice.

---

## Refactoring Recommendations

### R1 — Decompose `main.go` into packages (Highest ROI)

Extract the following responsibilities from `main.go`:

| New Package | Extracted From | Contents |
|---|---|---|
| `internal/converter/` | `main.go` | `processConversion()`, `buildConversionArgs()`, hash logic, output path construction |
| `internal/discovery/` | `main.go` | `producer()`, `walkDirectory()`, file filtering |
| `internal/app/` | `main.go` | `App` struct, startup wiring, flag parsing, drive enumeration |

After extraction, `main.go` becomes ~50 lines: parse flags → build `App` → call `app.Run()`.

---

### R2 — Consolidate quality tables

Create `internal/encoder/quality.go`:

```go
// QualityForResolution returns the recommended quality parameter for a given
// pixel count, using the standard 4K/1080p/720p/SD breakpoints.
func QualityForResolution(width, height int) int { ... }
```

All four encoder implementations call this function. Delete `config.DetermineQuality()`.

---

### R3 — Separate benchmark cache from config

Store `benchmark_cache.json` next to the config file. The cache key format (blake3 hash of probe command) is already stable — only the storage location changes. This eliminates the TOCTOU risk and keeps the user-facing config file clean.

---

### R4 — Introduce `App` struct to eliminate globals

```go
type App struct {
    Config   *types.Config
    DB       *database.Manager
    Registry *encoder.Registry
    Fallback *fallback.Manager
    Pipeline *pipeline.Pipeline
    UI       tui.Interface
    Log      *logrus.Logger
}

func (a *App) Run(ctx context.Context, paths []string) error { ... }
```

Every function that currently reads a global receives a pointer to `App` instead. Enables table-driven integration tests.

---

### R5 — Propagate context through conversion stack

```
main() ctx
  └── pipeline.Run(ctx)
        └── handler(ctx, job)  ← currently ignores ctx
              └── converter.Process(ctx, job)
                    └── ffmpeg.Run(ctx, args)  ← exec.CommandContext(ctx)
```

One change in `ffmpeg.Run` makes cancellation cross-platform and removes the dependency on `NtSuspendProcess` for stop (keep it only for pause).

---

## Scalability & Technical Debt

| Area | Current State | Risk |
|---|---|---|
| `main.go` size | 1,215 lines, growing each feature | High — every new feature adds to the blob |
| Test coverage of core logic | ~0% for `processConversion`, path logic, hash | High — any refactor is untested |
| Benchmark cache coupling | Lives inside config JSON | Medium — breaks on concurrent writes |
| `SaveAll()` per-job | O(n) writes for n files | Medium — degrades on large batches (1000+ files) |
| `SeqHook` unbounded timeout | 5s timeout × every log line | Medium — severe if Seq is misconfigured |
| Windows-only build | No build tags on `syscall`/`ntdll` | Low — documented, but CI on Linux would fail |
| `nvidia` package unused | Dead code in production | Low — maintenance burden |
| `NonInteractive` persisted | Surprising config mutation | Low — user confusion |

---

## Priority Action Plan

| Priority | Action | Effort | Impact | Status |
|---|---|---|---|---|
| 🔴 P1 | Extract `processConversion` + `buildConversionArgs` into `internal/converter/` | 3–4h | Unlocks testing of core logic | ✅ Done |
| 🔴 P1 | Introduce `App` struct, eliminate package-level globals | 2–3h | Unlocks integration testing | ✅ Done |
| 🟠 P2 | Consolidate quality tables → `internal/encoder/quality.go` | 1h | Eliminates 4× duplication | ✅ Done |
| 🟠 P2 | Separate benchmark cache from config file | 1h | Eliminates TOCTOU race | ✅ Done |
| 🟠 P2 | Propagate `ctx` into `processConversion` → `ffmpeg.Run` | 1h | Cross-platform cancellation | ✅ Done |
| 🟠 P2 | Fix `SaveAll()` — dirty-flag-only, flush on SIGINT | 1h | Major perf on large batches | ✅ Done |
| 🟡 P3 | Fix `FallbackManager` mutex-while-reading-stdin | 1h | Parallel-mode deadlock risk | ✅ Done |
| 🟡 P3 | Fix double `SetupLogging` call, leaked file handle | 30m | Resource leak | ✅ Done |
| 🟡 P3 | Add circuit breaker to `SeqHook` | 1h | Performance on bad Seq config | ✅ Done |
| 🟢 P4 | Encapsulate `Stats` mutations behind methods | 1h | Code hygiene | ✅ Done |
| 🟢 P4 | Fix `time.Sleep(50ms)` in `tui.New()` | 30m | Correctness / portability | ✅ Done |
| 🟢 P4 | Tag `NonInteractive` as `json:"-"` | 5m | Surprising behavior | ✅ Done |
| 🟢 P4 | Fix `freeBytes`/`availBytes` display | 15m | Accuracy on quota drives | ✅ Done |
| 🟢 P4 | Delete `config.DetermineQuality()` and `ffmpeg.BuildArgs()` | 15m | Remove dead code | ✅ Done |
