// Package app wires all subsystems together and exposes a single Run entry point.
// It replaces the package-level globals that previously lived in main.go.
package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	cfgpkg "video-converter/internal/config"
	"video-converter/internal/converter"
	"video-converter/internal/database"
	"video-converter/internal/discovery"
	"video-converter/internal/encoder"
	"video-converter/internal/fallback"
	"video-converter/internal/fileutil"
	"video-converter/internal/gpu/benchmark"
	"video-converter/internal/gpu/detect"
	"video-converter/internal/gpu/nvidia"
	"video-converter/internal/logging"
	"video-converter/internal/pipeline"
	"video-converter/internal/tui"
	"video-converter/internal/types"
)

const MaxParallelJobsCap = 8

// Options holds the parsed CLI flags.

type Options struct {
	ConfigFile     string
	DryRun         bool
	Bypass         bool
	ForceHevc      bool
	SameDrive      bool
	EncoderName    string
	ParallelJobs   int
	NonInteractive bool
	Rebenchmark    bool
	Paths          []string
}

// App holds all runtime state, replacing the package-level globals in main.go.
type App struct {
	opts            Options
	config          types.Config
	execDir         string
	configFilePath  string
	log             *logrus.Logger
	logCleanup      func()
	logFlush        func(serverURL, apiKey, execDir string, seqEnabled bool, w io.Writer) (*logrus.Logger, func())
	dbManager       *database.DatabaseManager
	stats           types.Stats
	selectedEncoder encoder.Encoder
	encoderRegistry *encoder.Registry
	fbManager       *fallback.FallbackManager
	ui              *tui.UI
	pipelineCtrl    *pipeline.Controller
	outputDrive     string          // "" = use source drive
	gpuBenchmarks   map[int]float64 // gpuIndex → FPS (nil for single-GPU or CPU)
}

// New creates an App from parsed options and validates basic preconditions.
func New(opts Options) (*App, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	return &App{
		opts:    opts,
		execDir: filepath.Dir(exePath),
	}, nil
}

// Run executes the full conversion workflow. It is the single entry point
// called by main after flag parsing.
func (a *App) Run() error {
	if err := a.loadConfig(); err != nil {
		return err
	}
	// Phase 1: early logger buffers in memory until the TUI is ready.
	a.log, a.logFlush = logging.SetupEarlyLogging(a.config.LogLevel)

	ffmpegExe := cfgpkg.ResolveExecutable(a.config.FFmpegPath, cfgpkg.ExeName("ffmpeg"), a.execDir)
	if ffmpegExe == "" {
		if err := a.promptInstallFFmpeg(); err != nil {
			return err
		}
	}

	a.fbManager = fallback.NewFallbackManager(!a.config.NonInteractive, os.Stdin, a.log)

	// ── Start the Bubble Tea program (setup phase first) ─────────────────────
	// The startup channel is closed once encoder detection + benchmarks finish,
	// which triggers the TUI to transition from the spinner screen to questions.
	startupCh := make(chan string, 16)

	setupOpts := tui.SetupOptions{
		StartupCh:        startupCh,
		NeedFolder:       len(a.opts.Paths) == 0,
		VideoExtensions:  a.config.FileExtensions,
		NeedBypass:       !a.opts.Bypass,
		NeedForceHevc:    !a.opts.ForceHevc,
		NeedParallelJobs: a.opts.ParallelJobs == 0,
		DefaultParallel:  a.config.MaxParallelJobs,
		NeedOutputDrive:  !a.opts.SameDrive,
		AvailableDrives:  getAvailableDrives(),
	}

	var pipeCtrl *pipeline.Controller
	var pipe *pipeline.Pipeline
	var answersCh <-chan tui.SetupAnswers

	a.ui, answersCh = tui.New(setupOpts, func(action tui.ControlAction) {
		if pipeCtrl == nil {
			return
		}
		switch action {
		case tui.ActionPause:
			if pipeCtrl.IsPaused() {
				pipeCtrl.Resume()
				a.ui.SendControlState(false, tui.StopKindNone)
			} else {
				pipeCtrl.Pause()
				a.ui.SendControlState(true, tui.StopKindNone)
			}
		case tui.ActionStopAfterCurrent:
			pipeCtrl.StopAfterCurrent()
			a.ui.SendControlState(false, tui.StopKindAfterCurrent)
		case tui.ActionStopNow:
			pipeCtrl.StopNow()
			if pipe != nil {
				pipe.Stop()
			}
			a.ui.SendControlState(false, tui.StopKindNow)
		}
	})

	// ── Encoder detection + benchmarks in background ─────────────────────────
	// Runs while the TUI shows the startup spinner; sends progress lines to
	// startupCh so the user can see what's happening.  Closing startupCh signals
	// that startup is complete and the TUI can show the setup questions.
	var (
		startupErrCh = make(chan error, 1)
	)
	go func() {
		defer close(startupCh)

		startupCh <- "Detecting encoder…"
		if err := a.selectEncoder(ffmpegExe); err != nil {
			startupErrCh <- err
			return
		}
		startupCh <- fmt.Sprintf("Encoder: %s", a.config.VideoEncoder)

		if err := a.runBenchmarks(ffmpegExe); err != nil {
			startupErrCh <- err
			return
		}
		// Update the DefaultParallel in opts now that benchmarks may have changed it.
		// We can't mutate setupOpts at this point, but the TUI will use the value
		// from answers.ParallelJobs if we set it; so store it for the hint below.
		startupCh <- fmt.Sprintf("Ready  (parallel jobs: %d)", a.config.MaxParallelJobs)
		startupErrCh <- nil
	}()

	// ── Wait for setup answers ───────────────────────────────────────────────
	answers := <-answersCh
	if answers.Cancelled {
		a.ui.Wait()
		// Drain the startup error channel so the goroutine can exit cleanly.
		go func() { <-startupErrCh }()
		return fmt.Errorf("setup cancelled by user")
	}

	// Check if the background startup goroutine hit an error.
	if err := <-startupErrCh; err != nil {
		a.ui.Wait()
		return err
	}

	// Update DefaultParallel hint in case benchmarks changed it.
	// (The user answered before benchmarks finished only if startup was very
	// fast and the hint was stale — acceptable; the benchmark result takes
	// priority below.)
	if answers.ParallelJobs > 0 {
		a.config.MaxParallelJobs = answers.ParallelJobs
	} else if a.opts.ParallelJobs > 0 {
		if a.opts.ParallelJobs > MaxParallelJobsCap {
			a.opts.ParallelJobs = MaxParallelJobsCap
		}
		a.config.MaxParallelJobs = a.opts.ParallelJobs
	}
	bypass := a.opts.Bypass || answers.Bypass
	forceHevc := a.opts.ForceHevc || answers.ForceHevc
	if answers.OutputDrive != "" {
		a.outputDrive = answers.OutputDrive
	}
	paths := a.opts.Paths
	if len(paths) == 0 {
		paths = answers.Paths
	}
	if len(paths) == 0 {
		a.ui.Wait()
		return fmt.Errorf("no input path provided")
	}
	// Phase 2: flush buffered early logs, open real log file, route through TUI.
	if a.logFlush != nil {
		a.log, a.logCleanup = a.logFlush(a.config.Seq.ServerURL, a.config.Seq.APIKey, a.execDir, a.config.Seq.Enabled, a.ui.Writer())
		a.logFlush = nil
	}

	a.dbManager = database.NewDatabaseManager(a.log)
	a.stats.TouchedDrives = make(map[string]bool)

	a.log.Info("Discovering files...")
	files, fileToBaseDir := discovery.DiscoverFiles(paths, a.config.FileExtensions, a.log)
	if len(files) == 0 {
		a.log.Info("No files found")
		return nil
	}
	a.log.Infof("Found %d files to process", len(files))

	numWorkers := a.config.MaxParallelJobs
	if numWorkers < 1 {
		numWorkers = 1
	}

	gpuAssigner := pipeline.NewGPUAssigner(a.gpuBenchmarks)
	if gpuAssigner != nil && gpuAssigner.NumGPUs() > 1 {
		numWorkers = gpuAssigner.NumGPUs() * a.config.MaxEncodesPerGPU
		if numWorkers > MaxParallelJobsCap {
			numWorkers = MaxParallelJobsCap
		}
		if numWorkers < 1 {
			numWorkers = 1
		}
		a.log.Infof("Multi-GPU: %d GPUs × %d encodes/GPU = %d workers", gpuAssigner.NumGPUs(), a.config.MaxEncodesPerGPU, numWorkers)
	}
	bufSize := numWorkers * 2
	if a.config.MaxQueueSize > bufSize {
		bufSize = a.config.MaxQueueSize
	}

	ctx := context.Background()
	pipe = pipeline.NewPipeline(ctx, numWorkers, bufSize)
	pipeCtrl = pipe.Controller()
	a.pipelineCtrl = pipeCtrl

	if numWorkers > 1 {
		a.log.Infof("Parallel mode: %d concurrent jobs", numWorkers)
	}

	conv := &converter.Converter{
		Config:              &a.config,
		ExecDir:             a.execDir,
		SelectedEncoder:     a.selectedEncoder,
		EncoderRegistry:     a.encoderRegistry,
		FallbackManager:     a.fbManager,
		DB:                  a.dbManager,
		UI:                  a.ui,
		Stats:               &a.stats,
		Ctrl:                pipeCtrl,
		OutputDriveOverride: a.outputDrive,
		Log:                 a.log,
	}

	pipe.Start(func(ctx context.Context, job types.Job) pipeline.ConversionResult {
		conv.Process(ctx, job, a.opts.DryRun)
		a.dbManager.SaveAll()
		return pipeline.ConversionResult{Job: job}
	})

	a.ui.StartTimer()

	go func() {
		discovery.Produce(files, fileToBaseDir, pipe, bypass, forceHevc, discovery.ProducerConfig{
			Config:      &a.config,
			ExecDir:     a.execDir,
			DBManager:   a.dbManager,
			Stats:       &a.stats,
			Log:         a.log,
			GPUAssigner: gpuAssigner,
		})
	}()

	for range pipe.Results() {
	}

	a.dbManager.SaveAll()

	elapsed := a.ui.Elapsed()
	tuiSummary := a.buildStatsSummary(elapsed)

	// Write the summary to the log file before tearing down the TUI so it is
	// always persisted regardless of how the user exits.
	for _, line := range tuiSummary {
		a.log.Info(line)
	}

	// Show the summary banner inside the TUI log viewport and wait for the user
	// to press Enter/q before tearing down the alt-screen.
	// In non-interactive / plain-text mode ShowSummary falls back to printing
	// directly and returns immediately.
	a.ui.ShowSummary(tuiSummary)

	// After ShowSummary the alt-screen is gone — safe to write to stdout.
	// Print the box-art version for readability in the plain terminal.
	boxSummary := a.buildStatsSummaryBox(elapsed)
	a.ui.PrintSummary(boxSummary)

	openHSortedFolders(a.stats.TouchedDrives)

	fmt.Println("All tasks completed")
	fmt.Println()
	if a.config.NonInteractive {
		fmt.Printf("ELAPSED=%s\n", fileutil.FmtElapsed(elapsed))
	}
	return nil
}

// ── private helpers ────────────────────────────────────────────────────────────

func (a *App) loadConfig() error {
	configPath := a.opts.ConfigFile
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(a.execDir, configPath)
	}
	cfg, err := cfgpkg.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	a.config = cfg
	a.configFilePath = configPath

	if a.opts.NonInteractive {
		a.config.NonInteractive = true
	}
	if a.opts.Rebenchmark {
		a.config.Rebenchmark = true
	}
	if a.opts.EncoderName != "auto" {
		a.config.VideoEncoder = a.opts.EncoderName
	} else if a.config.VideoEncoder == "" {
		a.config.VideoEncoder = "auto"
	}
	cfgpkg.ApplyGPUDefaults(&a.config)
	return nil
}

func (a *App) selectEncoder(ffmpegExe string) error {
	a.encoderRegistry = encoder.NewRegistry()
	a.encoderRegistry.Register(encoder.NewLibx265Encoder())
	a.encoderRegistry.Register(encoder.NewNvencEncoder())
	a.encoderRegistry.Register(encoder.NewAmfEncoder())
	a.encoderRegistry.Register(encoder.NewQsvEncoder())

	if a.config.VideoEncoder == "auto" {
		a.log.Info("Auto-detecting GPU encoders...")
		detectionResult, err := detect.DetectEncoders(ffmpegExe, a.log)
		if err != nil {
			a.log.Warnf("GPU detection failed: %v — falling back to libx265", err)
			a.config.VideoEncoder = "libx265"
		} else {
			best := detect.SelectBest(detectionResult)
			a.config.VideoEncoder = best.Encoder
			a.log.Infof("Detected GPU: %s (encoder: %s)", best.Name, best.Encoder)
			for _, gpu := range detectionResult.Available {
				status := "unavailable"
				if gpu.Available {
					status = "available"
				}
				a.log.Debugf("  %s (%s): %s", gpu.Name, gpu.Encoder, status)
			}
		}
	}

	enc, ok := a.encoderRegistry.Get(a.config.VideoEncoder)
	if !ok {
		a.log.Warnf("Unknown encoder %q, falling back to libx265", a.config.VideoEncoder)
		enc, _ = a.encoderRegistry.Get("libx265")
		a.config.VideoEncoder = "libx265"
	}
	a.selectedEncoder = enc
	a.log.Infof("Selected encoder: %s", a.config.VideoEncoder)
	return nil
}

func (a *App) runBenchmarks(ffmpegExe string) error {
	if a.config.VideoEncoder == "libx265" {
		return nil
	}

	qualityArgs := a.selectedEncoder.QualityArgs(a.config.QualityPreset, 1920)
	cache, _ := benchmark.LoadCache(a.configFilePath)
	if cache == nil {
		cache = &benchmark.BenchmarkCache{
			Results:         make(map[string]benchmark.BenchmarkResult),
			ParallelResults: make(map[string]benchmark.ParallelBenchmarkResult),
			Version:         "1",
		}
	}

	// Multi-GPU: enumerate NVIDIA GPUs and benchmark each one individually.
	if a.config.VideoEncoder == "hevc_nvenc" {
		gpus, err := nvidia.QueryGPUs(a.log)
		if err == nil && len(gpus) > 1 {
			a.log.Infof("Detected %d NVIDIA GPUs — benchmarking each", len(gpus))
			a.gpuBenchmarks = make(map[int]float64, len(gpus))

			for _, gpu := range gpus {
				deviceArgs := a.selectedEncoder.DeviceArgs(gpu.Index)
				key := benchmark.CacheKey(a.config.VideoEncoder, gpu.Name, "")

				if !a.config.Rebenchmark && benchmark.IsCacheValid(cache, key) {
					cached := cache.Results[key]
					a.log.Infof("Using cached benchmark for GPU %d (%s): %.1f FPS", gpu.Index, gpu.Name, cached.FPS)
					a.gpuBenchmarks[gpu.Index] = cached.FPS
					continue
				}

				a.log.Infof("Benchmarking GPU %d (%s, %d MB VRAM)...", gpu.Index, gpu.Name, gpu.VRAMTotalMB)
				result, err := benchmark.RunBenchmark(ffmpegExe, a.config.VideoEncoder, gpu.Index, qualityArgs, a.log, deviceArgs)
				if err != nil {
					a.log.Warnf("Benchmark failed for GPU %d (%s): %v — skipping this GPU", gpu.Index, gpu.Name, err)
					continue
				}
				a.log.Infof("GPU %d (%s): %.1f FPS (%.2fx realtime)", gpu.Index, gpu.Name, result.FPS, result.SpeedX)
				result.CacheKey = key
				cache.Results[key] = *result
				a.gpuBenchmarks[gpu.Index] = result.FPS
			}

			if len(a.gpuBenchmarks) == 0 {
				a.log.Warn("All GPU benchmarks failed — falling back to single-GPU mode")
				a.gpuBenchmarks = nil
			}
		}
	}

	// Single-GPU / non-NVENC benchmark (also runs when multi-GPU detection found only 1 GPU).
	singleKey := benchmark.CacheKey(a.config.VideoEncoder, "", "")
	if a.gpuBenchmarks == nil {
		if a.config.Rebenchmark || !benchmark.IsCacheValid(cache, singleKey) {
			a.log.Info("Running GPU benchmark (single stream)...")
			result, err := benchmark.RunBenchmark(ffmpegExe, a.config.VideoEncoder, 0, qualityArgs, a.log)
			if err != nil {
				a.log.Warnf("Benchmark failed: %v — proceeding without speed data", err)
			} else {
				a.log.Infof("Benchmark: %s @ %.1f FPS (%.2fx realtime)", a.config.VideoEncoder, result.FPS, result.SpeedX)
				result.CacheKey = singleKey
				cache.Results[singleKey] = *result
			}
		} else {
			cached := cache.Results[singleKey]
			a.log.Infof("Using cached benchmark: %.1f FPS for %s", cached.FPS, a.config.VideoEncoder)
		}
	}

	// Parallel sweep (single-GPU only — multi-GPU uses per-GPU benchmarks for worker count).
	if a.gpuBenchmarks == nil {
		parallelKey := benchmark.ParallelCacheKey(a.config.VideoEncoder)
		if a.config.Rebenchmark || !benchmark.IsParallelCacheValid(cache, parallelKey) {
			a.log.Info("Running parallel performance sweep (this takes ~2 minutes)...")
			sweepResult, err := benchmark.RunParallelSweep(ffmpegExe, a.config.VideoEncoder, 4, qualityArgs, a.log)
			if err != nil {
				a.log.Warnf("Parallel sweep failed: %v — keeping current parallel setting", err)
			} else {
				cache.ParallelResults[parallelKey] = *sweepResult
				a.config.MaxParallelJobs = sweepResult.BestParallelism
				a.log.Infof("Auto-configured: %d parallel job(s) gives best throughput (%.1f FPS)", sweepResult.BestParallelism, sweepResult.BestFPS)
			}
		} else {
			cached := cache.ParallelResults[parallelKey]
			a.log.Infof("Using cached parallel sweep: best = %d job(s) @ %.1f FPS", cached.BestParallelism, cached.BestFPS)
			if a.config.MaxParallelJobs <= 1 {
				a.config.MaxParallelJobs = cached.BestParallelism
			}
		}
	}

	if err := benchmark.SaveCache(a.configFilePath, cache); err != nil {
		a.log.Warnf("Failed to save benchmark cache: %v", err)
	}
	return nil
}

func (a *App) buildStatsSummary(elapsed time.Duration) []string {
	a.stats.Mu.Lock()
	defer a.stats.Mu.Unlock()

	var lines []string
	lines = append(lines,
		"",
		"  VIDEO CONVERSION SUMMARY",
		"  ─────────────────────────────────────",
		fmt.Sprintf("  Files Analyzed:     %d", a.stats.FilesAnalyzed),
		fmt.Sprintf("  Files Converted:    %d", a.stats.FilesProcessed),
		fmt.Sprintf("    → Improved:       %d", a.stats.FilesImproved),
		fmt.Sprintf("    → Discarded:      %d", a.stats.FilesDiscarded),
		fmt.Sprintf("  Files Skipped:      %d", a.stats.FilesSkipped),
		fmt.Sprintf("  Files Errored:      %d", a.stats.FilesErrored),
		"  ─────────────────────────────────────",
	)

	if a.stats.OriginalBytes > 0 {
		saved := a.stats.OriginalBytes - a.stats.FinalBytes
		pct := float64(saved) / float64(a.stats.OriginalBytes) * 100
		lines = append(lines,
			fmt.Sprintf("  Original Size:      %s", fileutil.FormatBytes(a.stats.OriginalBytes)),
			fmt.Sprintf("  Final Size:         %s", fileutil.FormatBytes(a.stats.FinalBytes)),
			fmt.Sprintf("  Space Saved:        %s  (%.1f%%)", fileutil.FormatBytes(saved), pct),
		)
	}

	if elapsed > 0 {
		lines = append(lines,
			"  ─────────────────────────────────────",
			fmt.Sprintf("  Total Time:         %s", fileutil.FmtElapsed(elapsed)),
		)
	}

	lines = append(lines, "")
	return lines
}

// buildStatsSummaryBox returns the same data formatted as a fixed-width box for
// the post-TUI plain-text stdout print.
func (a *App) buildStatsSummaryBox(elapsed time.Duration) []string {
	a.stats.Mu.Lock()
	defer a.stats.Mu.Unlock()

	var lines []string
	lines = append(lines,
		"",
		"╔════════════════════════════════════════════════════════════════╗",
		"║           VIDEO CONVERSION SUMMARY                             ║",
		"╠════════════════════════════════════════════════════════════════╣",
		fmt.Sprintf("║ Files Analyzed:     %-43d║", a.stats.FilesAnalyzed),
		fmt.Sprintf("║ Files Converted:    %-43d║", a.stats.FilesProcessed),
		fmt.Sprintf("║   → Improved:       %-43d║", a.stats.FilesImproved),
		fmt.Sprintf("║   → Discarded:      %-43d║", a.stats.FilesDiscarded),
		fmt.Sprintf("║ Files Skipped:      %-43d║", a.stats.FilesSkipped),
		fmt.Sprintf("║ Files Errored:      %-43d║", a.stats.FilesErrored),
		"╠════════════════════════════════════════════════════════════════╣",
	)

	if a.stats.OriginalBytes > 0 {
		saved := a.stats.OriginalBytes - a.stats.FinalBytes
		pct := float64(saved) / float64(a.stats.OriginalBytes) * 100
		lines = append(lines,
			fmt.Sprintf("║ Original Size:      %-43s║", fileutil.FormatBytes(a.stats.OriginalBytes)),
			fmt.Sprintf("║ Final Size:         %-43s║", fileutil.FormatBytes(a.stats.FinalBytes)),
			fmt.Sprintf("║ Space Saved:        %-33s (%.1f%%)║", fileutil.FormatBytes(saved), pct),
		)
	}

	if elapsed > 0 {
		lines = append(lines,
			"╠════════════════════════════════════════════════════════════════╣",
			fmt.Sprintf("║ Total Time:         %-43s║", fileutil.FmtElapsed(elapsed)),
		)
	}

	lines = append(lines,
		"╚════════════════════════════════════════════════════════════════╝",
		"",
	)
	return lines
}

// ── interactive prompt helpers ─────────────────────────────────────────────────

// promptInstallFFmpeg informs the user that ffmpeg was not found and offers to
// open the download page in their browser. It always returns a non-nil error so
// that Run() exits cleanly after the prompt.
func (a *App) promptInstallFFmpeg() error {
	const downloadURL = "https://www.ffmpeg.org/download.html"

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "ERROR: ffmpeg was not found on this system.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "ffmpeg is required to convert videos. You can download it from:")
	fmt.Fprintf(os.Stderr, "  %s\n", downloadURL)
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Please install ffmpeg and add it to PATH, or set ffmpeg_path in the config file.")
	if !a.config.NonInteractive {
		if err := openURL(downloadURL); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open browser: %v\n", err)
		}
	}

	return fmt.Errorf("ffmpeg not found — please install ffmpeg and add it to PATH, or set ffmpeg_path in the config file")
}

func openHSortedFolders(touchedDrives map[string]bool) {
	for drive := range touchedDrives {
		hsortedPath := filepath.Join(drive, converter.OutputDirName)
		if _, err := os.Stat(hsortedPath); err == nil {
			openFolder(hsortedPath) //nolint:errcheck
		}
	}
}

// ExitWithPause pauses before exit so users can read the last output.
func ExitWithPause(code int) {
	fmt.Fprintln(os.Stderr, "\nPress Enter to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n') //nolint:errcheck
	os.Exit(code)
}
