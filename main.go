package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"

	cfgpkg "video-converter/internal/config"
	"video-converter/internal/database"
	"video-converter/internal/encoder"
	"video-converter/internal/fallback"
	"video-converter/internal/ffmpeg"
	"video-converter/internal/gpu/benchmark"
	"video-converter/internal/gpu/detect"
	"video-converter/internal/logging"
	"video-converter/internal/pipeline"
	"video-converter/internal/tui"
	"video-converter/internal/types"
)

var log = logrus.New()

var (
	config              types.Config
	dbManager           *database.DatabaseManager
	stats               types.Stats
	execDir             string
	outputDriveOverride string // If set, use this drive instead of source drive
	selectedEncoder     encoder.Encoder
	encoderRegistry     *encoder.Registry
	fbManager           *fallback.FallbackManager
	configFilePath      string // saved for benchmark cache operations
	ui                  *tui.UI
)

func exitWithPause(code int) {
	fmt.Fprintln(os.Stderr, "\nPress Enter to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	os.Exit(code)
}

func main() {
	// Parse flags
	var (
		configFile     = flag.String("config", "configVideoConversion.json", "Path to config file")
		dryRun         = flag.Bool("dry-run", false, "Dry run mode")
		bypassFlag     = flag.Bool("bypass", false, "Re-convert files already in the database (bypass DB check)")
		forceHevcFlag  = flag.Bool("force-hevc", false, "Re-compress files that are already H.265/HEVC")
		sameDrive      = flag.Bool("same-drive", false, "Write output to the same drive as the source (skip drive prompt)")
		encoderFlag    = flag.String("encoder", "auto", "Video encoder (auto, hevc_nvenc, hevc_amf, hevc_qsv, libx265)")
		parallelJobs   = flag.Int("jobs", 0, "Number of parallel conversion jobs (0 = use benchmark recommendation)")
		nonInteractive = flag.Bool("non-interactive", false, "Disable interactive prompts for GPU fallback")
		rebenchmark    = flag.Bool("rebenchmark", false, "Force GPU benchmark even if cache exists")
	)
	flag.Parse()

	paths := flag.Args()

	bypass := *bypassFlag
	forceHevc := *forceHevcFlag

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		exitWithPause(1)
	}
	execDir = filepath.Dir(exePath)

	// Load or create config
	configPath := *configFile
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(execDir, configPath)
	}
	cfg, err := cfgpkg.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		exitWithPause(1)
	}
	config = cfg

	// Apply CLI flag overrides to config
	if *nonInteractive {
		config.NonInteractive = true
	}
	if *rebenchmark {
		config.Rebenchmark = true
	}

	// Override encoder if explicitly specified via CLI flag
	if *encoderFlag != "auto" {
		config.VideoEncoder = *encoderFlag
	} else if config.VideoEncoder == "" {
		config.VideoEncoder = "auto"
	}

	// Apply GPU defaults to config
	cfgpkg.ApplyGPUDefaults(&config)

	// Save config path for benchmark cache
	configFilePath = configPath

	// Setup logging to stdout before TUI starts (prompts happen on plain terminal)
	var logCleanup func()
	log, logCleanup = logging.SetupLogging(config.ServerURL, config.APIKey, config.LogLevel, execDir, nil)
	defer logCleanup()

	// Resolve FFmpeg path for GPU detection
	ffmpegExe := cfgpkg.ResolveExecutable(config.FFmpegPath, "ffmpeg.exe", execDir)

	// GPU detection and encoder selection
	encoderRegistry = encoder.NewRegistry()
	encoderRegistry.Register(encoder.NewLibx265Encoder())
	encoderRegistry.Register(encoder.NewNvencEncoder())
	encoderRegistry.Register(encoder.NewAmfEncoder())
	encoderRegistry.Register(encoder.NewQsvEncoder())

	if config.VideoEncoder == "auto" {
		log.Info("Auto-detecting GPU encoders...")
		detectionResult, err := detect.DetectEncoders(ffmpegExe, log)
		if err != nil {
			log.Warnf("GPU detection failed: %v — falling back to libx265", err)
			config.VideoEncoder = "libx265"
		} else {
			best := detect.SelectBest(detectionResult)
			config.VideoEncoder = best.Encoder
			log.Infof("Detected GPU: %s (encoder: %s)", best.Name, best.Encoder)

			// Log all detected encoders
			for _, gpu := range detectionResult.Available {
				status := "unavailable"
				if gpu.Available {
					status = "available"
				}
				log.Debugf("  %s (%s): %s", gpu.Name, gpu.Encoder, status)
			}
		}
	}

	if enc, ok := encoderRegistry.Get(config.VideoEncoder); ok {
		selectedEncoder = enc
	} else {
		log.Warnf("Unknown encoder %q, falling back to libx265", config.VideoEncoder)
		selectedEncoder, _ = encoderRegistry.Get("libx265")
		config.VideoEncoder = "libx265"
	}
	log.Infof("Selected encoder: %s", config.VideoEncoder)

	// Benchmark (for GPU encoders only)
	if config.VideoEncoder != "libx265" {
		qualityArgs := selectedEncoder.QualityArgs(config.QualityPreset, 1920)
		singleKey := benchmark.CacheKey(config.VideoEncoder, "", "")
		parallelKey := benchmark.ParallelCacheKey(config.VideoEncoder)

		cache, _ := benchmark.LoadCache(configFilePath)
		if cache == nil {
			cache = &benchmark.BenchmarkCache{
				Results:         make(map[string]benchmark.BenchmarkResult),
				ParallelResults: make(map[string]benchmark.ParallelBenchmarkResult),
				Version:         "1",
			}
		}

		// Run single-stream benchmark if missing/stale/forced
		if config.Rebenchmark || !benchmark.IsCacheValid(cache, singleKey) {
			log.Info("Running GPU benchmark (single stream)...")
			result, err := benchmark.RunBenchmark(ffmpegExe, config.VideoEncoder, 0, qualityArgs, log)
			if err != nil {
				log.Warnf("Benchmark failed: %v — proceeding without speed data", err)
			} else {
				log.Infof("Benchmark: %s @ %.1f FPS (%.2fx realtime)", config.VideoEncoder, result.FPS, result.SpeedX)
				result.CacheKey = singleKey
				cache.Results[singleKey] = *result
			}
		} else {
			cached := cache.Results[singleKey]
			log.Infof("Using cached benchmark: %.1f FPS for %s", cached.FPS, config.VideoEncoder)
		}

		// Run parallel sweep if missing/stale/forced
		if config.Rebenchmark || !benchmark.IsParallelCacheValid(cache, parallelKey) {
			log.Info("Running parallel performance sweep (this takes ~2 minutes)...")
			sweepResult, err := benchmark.RunParallelSweep(ffmpegExe, config.VideoEncoder, 4, qualityArgs, log)
			if err != nil {
				log.Warnf("Parallel sweep failed: %v — keeping current parallel setting", err)
			} else {
				cache.ParallelResults[parallelKey] = *sweepResult
				config.MaxParallelJobs = sweepResult.BestParallelism
				log.Infof("Auto-configured: %d parallel job(s) gives best throughput (%.1f FPS)", sweepResult.BestParallelism, sweepResult.BestFPS)
			}
		} else {
			cached := cache.ParallelResults[parallelKey]
			log.Infof("Using cached parallel sweep: best = %d job(s) @ %.1f FPS", cached.BestParallelism, cached.BestFPS)
			if config.MaxParallelJobs <= 1 {
				// Only apply cached recommendation if user hasn't manually set a higher value
				config.MaxParallelJobs = cached.BestParallelism
			}
		}

		// Persist updated cache
		if err := benchmark.SaveCache(configFilePath, cache); err != nil {
			log.Warnf("Failed to save benchmark cache: %v", err)
		}
	}

	// Create FallbackManager for GPU error recovery
	fbManager = fallback.NewFallbackManager(!config.NonInteractive, os.Stdin, log)

	// Interactive prompts — skipped when the corresponding flag is provided
	if !*bypassFlag {
		bypass = askYesNo("Force re-conversion (bypass DB check)? [y/N]: ")
	}
	if !*forceHevcFlag {
		forceHevc = askYesNo("Test re-compression even if file is already HEVC? [y/N]: ")
	}

	// Allow user to override the auto-detected parallel job count
	if *parallelJobs > 0 {
		if *parallelJobs > 8 {
			fmt.Println("Warning: values above 8 are unlikely to help and may thrash the GPU. Capping at 8.")
			*parallelJobs = 8
		}
		config.MaxParallelJobs = *parallelJobs
	} else {
		config.MaxParallelJobs = askParallelJobs(config.MaxParallelJobs)
	}

	// Ask about output drive (skipped when --same-drive is set)
	if *sameDrive {
		outputDriveOverride = "" // explicit: use source drive
	} else {
		outputDriveOverride = askOutputDrive()
	}

	// Prompt for folder path if none provided
	if len(paths) == 0 {
		reader := bufio.NewReader(os.Stdin)
		for attempts := 0; attempts < 3; attempts++ {
			fmt.Print("No input folder specified. Enter folder path (or drag a folder here): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
				continue
			}
			input = strings.Trim(strings.TrimSpace(input), "\"")
			if input == "" {
				fmt.Fprintln(os.Stderr, "Error: empty path provided")
				continue
			}
			info, err := os.Stat(input)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: path does not exist: %v\n", err)
				continue
			}
			if !info.IsDir() {
				fmt.Fprintln(os.Stderr, "Error: path is not a directory")
				continue
			}
			paths = []string{input}
			break
		}
		if len(paths) == 0 {
			log.Error("No valid input path provided after 3 attempts")
			exitWithPause(1)
		}
	}

	// All interactive prompts are done — start the TUI and re-route logging through it.
	ui = tui.New()
	log, logCleanup = logging.SetupLogging(config.ServerURL, config.APIKey, config.LogLevel, execDir, ui.Writer())

	dbManager = database.NewDatabaseManager(log)

	// Initialize stats
	stats.TouchedDrives = make(map[string]bool)

	// Discover files
	log.Info("Discovering files...")
	files, fileToBaseDir := discoverFiles(paths)
	if len(files) == 0 {
		log.Info("No files found")
		return
	}
	log.Infof("Found %d files to process", len(files))

	// Setup pipeline
	numWorkers := config.MaxParallelJobs
	if numWorkers < 1 {
		numWorkers = 1
	}
	bufSize := numWorkers * 2
	if config.MaxQueueSize > bufSize {
		bufSize = config.MaxQueueSize
	}

	ctx := context.Background()
	pipe := pipeline.NewPipeline(ctx, numWorkers, bufSize)

	if numWorkers > 1 {
		log.Infof("Parallel mode: %d concurrent jobs", numWorkers)
	}

	pipe.Start(func(ctx context.Context, job types.Job) pipeline.ConversionResult {
		processConversion(job, *dryRun)
		dbManager.SaveAll()
		return pipeline.ConversionResult{Job: job}
	})

	// Start global session timer (all prompts are done at this point)
	ui.StartTimer()

	// Produce jobs then signal done (closes jobs channel + results when workers finish)
	go func() {
		producer(files, fileToBaseDir, pipe, bypass, forceHevc)
	}()

	// Drain results — blocks until pipeline is fully drained and closed
	for range pipe.Results() {
	}

	// Save all databases
	dbManager.SaveAll()

	// Stop TUI (restores normal screen) then print summary to normal terminal
	elapsed := ui.Elapsed()
	ui.Wait()
	ui.PrintSummary(buildStatsSummary(elapsed))

	// Auto-open HSORTED folders
	openHSortedFolders()

	fmt.Println("All tasks completed")
	fmt.Println()
	if config.NonInteractive {
		// Machine-readable line for scripting (e.g. batch benchmark)
		fmt.Printf("ELAPSED=%s\n", fmtElapsed(elapsed))
	} else {
		fmt.Println("Press Enter to exit...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}
}

func askYesNo(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		// EOF or error, assume No
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

// askParallelJobs asks how many jobs to run concurrently.
// The recommended value is pre-filled from the parallel benchmark sweep.
// Returns the chosen value (1 = sequential).
func askParallelJobs(recommended int) int {
	fmt.Printf("\nParallel jobs [recommended: %d, Enter to accept]: ", recommended)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return recommended
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return recommended
	}
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 {
		fmt.Println("Invalid input, using recommended value.")
		return recommended
	}
	if n > 8 {
		fmt.Println("Warning: values above 8 are unlikely to help and may thrash the GPU. Capping at 8.")
		n = 8
	}
	return n
}

// getAvailableDrives returns a list of available drive letters on Windows
func getAvailableDrives() []string {
	var drives []string
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drivePath := string(drive) + ":\\"
		if _, err := os.Stat(drivePath); err == nil {
			// Get drive space info
			var freeBytes, totalBytes, availBytes uint64
			kernel32 := syscall.NewLazyDLL("kernel32.dll")
			getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

			driveName, _ := syscall.UTF16PtrFromString(drivePath)
			ret, _, _ := getDiskFreeSpaceEx.Call(
				uintptr(unsafe.Pointer(driveName)),
				uintptr(unsafe.Pointer(&availBytes)),
				uintptr(unsafe.Pointer(&totalBytes)),
				uintptr(unsafe.Pointer(&freeBytes)),
			)

			if ret != 0 {
				freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
				totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
				drives = append(drives, fmt.Sprintf("%s: (%.1f GB free / %.1f GB total)", drivePath, freeGB, totalGB))
			} else {
				drives = append(drives, drivePath)
			}
		}
	}
	return drives
}

// askOutputDrive asks user if they want to use a different output drive
func askOutputDrive() string {
	if !askYesNo("Use a different drive for output? [y/N]: ") {
		return "" // Use source drive
	}

	fmt.Println("\nAvailable drives:")
	drives := getAvailableDrives()
	if len(drives) == 0 {
		fmt.Println("No drives found!")
		return ""
	}

	for i, drive := range drives {
		fmt.Printf("  %d) %s\n", i+1, drive)
	}

	fmt.Print("\nSelect drive number (or Enter to cancel): ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return ""
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return ""
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(drives) {
		fmt.Println("Invalid selection, using source drive")
		return ""
	}

	// Extract drive letter from the selection (e.g., "D:\" from "D:\ (123.4 GB free / 500.0 GB total)")
	selectedDrive := drives[choice-1]
	driveLetter := strings.Split(selectedDrive, " ")[0]

	fmt.Printf("Output will be written to: %s\n", driveLetter)
	return driveLetter
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// discoverFiles returns a map of file paths to their base directories.
// The base directory is the user-provided input path (dropped directory).
func discoverFiles(paths []string) ([]string, map[string]string) {
	var files []string
	fileToBaseDir := make(map[string]string)
	extMap := make(map[string]bool)
	for _, ext := range config.FileExtensions {
		extMap[strings.ToLower(ext)] = true
	}

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			log.Warnf("Failed to get absolute path for %s: %v", p, err)
			continue
		}

		info, err := os.Stat(absPath)
		if err != nil {
			log.Warnf("Failed to stat %s: %v", absPath, err)
			continue
		}

		if info.IsDir() {
			// For directories, the base is the directory itself
			filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() && extMap[strings.ToLower(filepath.Ext(path))] {
					files = append(files, path)
					fileToBaseDir[path] = absPath
				}
				return nil
			})
		} else {
			// For single files, the base is the parent directory
			if extMap[strings.ToLower(filepath.Ext(absPath))] {
				files = append(files, absPath)
				fileToBaseDir[absPath] = filepath.Dir(absPath)
			}
		}
	}

	return files, fileToBaseDir
}

func resolveFFprobeExe() string {
	ffprobeExe := cfgpkg.ResolveExecutable(config.FFprobePath, "ffprobe.exe", execDir)
	if ffprobeExe == "" {
		ffmpegExe := cfgpkg.ResolveExecutable(config.FFmpegPath, "ffmpeg.exe", execDir)
		ffprobeExe = filepath.Join(filepath.Dir(ffmpegExe), "ffprobe.exe")
		if _, err := os.Stat(ffprobeExe); err != nil {
			ffprobeExe, _ = exec.LookPath("ffprobe.exe")
		}
	}
	return ffprobeExe
}

func producer(files []string, fileToBaseDir map[string]string, pipe *pipeline.Pipeline, bypass, forceHevc bool) {
	defer pipe.Wait()

	// Disable progress bar to avoid conflicts with log output
	// bar := progressbar.Default(int64(len(files)), "Processing")

	// Group files by parent folder
	folderMap := make(map[string][]string)
	for _, filePath := range files {
		driveRoot := getDriveRoot(filePath)
		parentFolder := getParentFolderName(filePath, driveRoot)
		folderMap[parentFolder] = append(folderMap[parentFolder], filePath)
	}

	// Get sorted list of folders
	var folders []string
	for folder := range folderMap {
		folders = append(folders, folder)
	}
	sort.Strings(folders)

	totalFolders := len(folders)
	totalFiles := len(files)
	globalFileNumber := 0

	for folderIdx, folder := range folders {
		folderNumber := folderIdx + 1
		filesInFolder := folderMap[folder]

		log.Infof("\nProcessing folder %d/%d: %s (%d files)", folderNumber, totalFolders, folder, len(filesInFolder))

		for _, filePath := range filesInFolder {
			globalFileNumber++
			log.Debugf("[%d/%d] Analyzing: %s", globalFileNumber, totalFiles, filepath.Base(filePath))

			stats.Mu.Lock()
			stats.FilesAnalyzed++
			stats.Mu.Unlock()

			// Get file info
			info, err := os.Stat(filePath)
			if err != nil {
				log.Warnf("Failed to stat %s: %v", filePath, err)
				stats.Mu.Lock()
				stats.FilesErrored++
				stats.Mu.Unlock()
				continue
			}

			// Get drive root
			driveRoot := getDriveRoot(filePath)
			stats.Mu.Lock()
			stats.TouchedDrives[driveRoot] = true
			stats.Mu.Unlock()

			// Compute hash
			fileHash, _ := getFileHash(filePath, config.UsePartialHash)
			if fileHash == "error_hash" {
				log.Warnf("Failed to hash %s", filePath)
				stats.Mu.Lock()
				stats.FilesErrored++
				stats.Mu.Unlock()
				continue
			}

			// Check cache
			if !bypass {
				rec := dbManager.GetRecord(driveRoot, fileHash)
				if rec != nil && (rec.Output != "" || rec.Note == "not_beneficial" || rec.Note == "already_hevc") {
					log.Debugf("Skipping %s (already processed)", filePath)
					stats.Mu.Lock()
					stats.FilesSkipped++
					stats.Mu.Unlock()
					continue
				}
			}

			// Get video info via ffprobe
			ffprobeExe := resolveFFprobeExe()
			videoInfo, err := ffmpeg.GetMediaInfo(filePath, ffprobeExe)
			if err != nil {
				log.Warnf("Failed to get video info for %s: %v", filePath, err)
				stats.Mu.Lock()
				stats.FilesErrored++
				stats.Mu.Unlock()
				continue
			}
			if videoInfo == nil {
				log.Warnf("No video track found in %s", filePath)
				stats.Mu.Lock()
				stats.FilesErrored++
				stats.Mu.Unlock()
				continue
			}

			// Check if already HEVC
			if ffmpeg.IsHEVC(videoInfo.Format, videoInfo.CodecID) && !forceHevc {
				log.Infof("Skipping %s (already HEVC)", filePath)
				dbManager.UpdateRecord(driveRoot, fileHash, types.Record{
					OriginalSize:  info.Size(),
					ConvertedSize: info.Size(),
					Note:          "already_hevc",
				})
				stats.Mu.Lock()
				stats.FilesSkipped++
				stats.Mu.Unlock()
				continue
			}

			// Get duration via FFprobe
			ffprobeExe = resolveFFprobeExe()
			duration := ffmpeg.GetDuration(filePath, ffprobeExe, log)

			// Enqueue job
			job := types.Job{
				FilePath:        filePath,
				BaseDir:         fileToBaseDir[filePath],
				DriveRoot:       driveRoot,
				FileHash:        fileHash,
				OriginalSize:    info.Size(),
				Width:           videoInfo.Width,
				Height:          videoInfo.Height,
				Format:          videoInfo.Format,
				CodecID:         videoInfo.CodecID,
				DurationSeconds: duration,
				FileNumber:      globalFileNumber,
				TotalFiles:      totalFiles,
				FolderNumber:    folderNumber,
				TotalFolders:    totalFolders,
			}
			pipe.Submit(job) //nolint:errcheck // context cancellation is benign here
		}
	}
}

func processConversion(job types.Job, dryRun bool) {
	fileName := filepath.Base(job.FilePath)

	log.Infof("Converting [%d/%d]: %s (%dp)", job.FileNumber, job.TotalFiles, fileName, job.Height)
	if strings.ToLower(filepath.Ext(job.FilePath)) == ".mkv" {
		log.Infof("  Format: MKV (preserving all audio/subtitle streams & metadata)")
	}

	if dryRun {
		log.Infof("[DRY RUN] Would convert: %s (%dp)", fileName, job.Height)
		return
	}

	// Register a TUI progress bar for this job.
	jobID := ui.StartJob(fileName, job.FileNumber, job.TotalFiles, job.Height, job.DurationSeconds)
	// onProgress callback: forward FFmpeg progress to the TUI bar.
	onProgress := func(pct int) {
		ui.UpdateProgress(jobID, pct)
	}

	// Determine output paths - preserve directory structure from base directory
	// Example: If BaseDir = C:\Temp\a and FilePath = C:\Temp\a\b\c\video.mkv
	// then baseDirName = "a", relPath = "b\c", and output = D:\HSORTED\a\b\c\video.mp4

	baseDirName := filepath.Base(job.BaseDir)
	fileDir := filepath.Dir(job.FilePath)

	// Get relative path from base directory to file's directory
	relPath, err := filepath.Rel(job.BaseDir, fileDir)
	if err != nil || relPath == "." {
		// File is directly in base directory
		relPath = ""
	}

	// Combine base directory name with relative path
	var fullRelPath string
	if relPath == "" {
		fullRelPath = baseDirName
	} else {
		fullRelPath = filepath.Join(baseDirName, relPath)
	}

	sanitized := sanitizeFolderName(fullRelPath)

	// Use output drive override if specified, otherwise use source drive
	outputRoot := job.DriveRoot
	if outputDriveOverride != "" {
		outputRoot = outputDriveOverride
	}

	finalDir := filepath.Join(outputRoot, "HSORTED", sanitized)
	// Don't create finalDir yet - only create it if conversion succeeds

	// Determine temp directory
	var tempDir string
	if config.TempDirectory != "" {
		if _, err := os.Stat(config.TempDirectory); err == nil {
			tempDir = filepath.Join(config.TempDirectory, sanitized)
		}
	}
	if tempDir == "" {
		tempDir = filepath.Join(outputRoot, "HSORTED", "_TEMP", sanitized)
	}
	// Create temp directory only when needed
	os.MkdirAll(tempDir, 0755)

	hash8 := job.FileHash[:8]

	// Determine output extension based on input format
	inputExt := strings.ToLower(filepath.Ext(job.FilePath))
	outputExt := ".mp4"
	if inputExt == ".mkv" {
		outputExt = ".mkv"
	}

	tempPath := filepath.Join(tempDir, fmt.Sprintf("__tmp__%s%s", hash8, outputExt))

	// Build FFmpeg command using encoder's quality/device args
	qualityArgs := selectedEncoder.QualityArgs(config.QualityPreset, job.Width)
	deviceArgs := selectedEncoder.DeviceArgs(job.GPUIndex)
	args := buildConversionArgs(job.FilePath, tempPath, outputExt, config.VideoEncoder, qualityArgs, deviceArgs)
	// Run FFmpeg
	ffmpegExe := cfgpkg.ResolveExecutable(config.FFmpegPath, "ffmpeg.exe", execDir)
	rc, stderrOut := ffmpeg.Run(ffmpegExe, args, job.FilePath, job.DurationSeconds, log, onProgress)
	// Handle GPU failure with fallback
	if rc != 0 && config.VideoEncoder != "libx265" {
		shouldFallback, fbErr := fbManager.HandleGPUError(stderrOut, selectedEncoder, &jobStringer{job})
		if fbErr != nil {
			log.Warnf("Fallback error: %v", fbErr)
		}
		if shouldFallback {
			log.Info("Falling back to CPU encoder (libx265)...")
			cpuEncoder, _ := encoderRegistry.Get("libx265")
			cpuQualityArgs := cpuEncoder.QualityArgs(config.QualityPreset, job.Width)
			cpuArgs := buildConversionArgs(job.FilePath, tempPath, outputExt, "libx265", cpuQualityArgs, nil)
			rc, stderrOut = ffmpeg.Run(ffmpegExe, cpuArgs, job.FilePath, job.DurationSeconds, log, onProgress)
		}
	}
	if rc != 0 {
		log.Errorf("FFmpeg failed with exit code %d for %s", rc, job.FilePath)
		if stderrOut != "" {
			log.Errorf("FFmpeg stderr: %s", stderrOut)
		}
		ui.CompleteError(jobID, fmt.Sprintf("✗ FAILED  [%d/%d] %s", job.FileNumber, job.TotalFiles, fileName))
		dbManager.UpdateRecord(job.DriveRoot, job.FileHash, types.Record{
			OriginalSize: job.OriginalSize,
			Error:        fmt.Sprintf("rc_%d", rc),
		})
		os.Remove(tempPath)
		stats.Mu.Lock()
		stats.FilesErrored++
		stats.Mu.Unlock()
		return
	}

	// Check output size
	tempInfo, err := os.Stat(tempPath)
	if err != nil {
		log.Errorf("Failed to stat temp file: %v", err)
		ui.CompleteError(jobID, fmt.Sprintf("✗ ERROR   [%d/%d] %s", job.FileNumber, job.TotalFiles, fileName))
		os.Remove(tempPath)
		return
	}

	newSize := tempInfo.Size()

	origMB := float64(job.OriginalSize) / (1024 * 1024)
	newMB := float64(newSize) / (1024 * 1024)

	if newSize < job.OriginalSize {
		// KEPT - File improved
		reduction := float64(job.OriginalSize-newSize) / float64(job.OriginalSize) * 100
		savedMB := origMB - newMB
		summary := fmt.Sprintf("✓ KEPT    [%d/%d] %-32s  %.2f MB → %.2f MB  (-%.2f MB, %.1f%%)",
			job.FileNumber, job.TotalFiles, truncateString(fileName, 32), origMB, newMB, savedMB, reduction)
		ui.CompleteKept(jobID, summary)

		// Create final directory now that we know the file will be kept
		os.MkdirAll(finalDir, 0755)

		// Move to final location
		baseName := strings.TrimSuffix(filepath.Base(job.FilePath), filepath.Ext(job.FilePath))

		// Determine output extension based on input format
		inputExt := strings.ToLower(filepath.Ext(job.FilePath))
		outputExt := ".mp4"
		if inputExt == ".mkv" {
			outputExt = ".mkv"
		}

		finalPath := filepath.Join(finalDir, baseName+outputExt)
		if _, err := os.Stat(finalPath); err == nil {
			finalPath = filepath.Join(finalDir, fmt.Sprintf("%s__%s%s", baseName, hash8, outputExt))
		}

		if err := os.Rename(tempPath, finalPath); err != nil {
			log.Errorf("Failed to move file: %v", err)
			os.Remove(tempPath)
			return
		}

		dbManager.UpdateRecord(job.DriveRoot, job.FileHash, types.Record{
			OriginalSize:  job.OriginalSize,
			ConvertedSize: newSize,
			Output:        finalPath,
		})

		stats.Mu.Lock()
		stats.FilesProcessed++
		stats.FilesImproved++
		stats.OriginalBytes += job.OriginalSize
		stats.FinalBytes += newSize
		stats.Mu.Unlock()
	} else {
		// DISCARDED - File not improved
		increase := float64(newSize-job.OriginalSize) / float64(job.OriginalSize) * 100
		increasedMB := newMB - origMB
		summary := fmt.Sprintf("✗ DISCARD [%d/%d] %-32s  %.2f MB → %.2f MB  (+%.2f MB, +%.1f%%)",
			job.FileNumber, job.TotalFiles, truncateString(fileName, 32), origMB, newMB, increasedMB, increase)
		ui.CompleteDiscard(jobID, summary)

		os.Remove(tempPath)

		dbManager.UpdateRecord(job.DriveRoot, job.FileHash, types.Record{
			OriginalSize:  job.OriginalSize,
			ConvertedSize: newSize,
			Note:          "not_beneficial",
		})

		stats.Mu.Lock()
		stats.FilesProcessed++
		stats.FilesDiscarded++
		stats.OriginalBytes += job.OriginalSize
		stats.FinalBytes += job.OriginalSize
		stats.Mu.Unlock()
	}
}

func getFileHash(filePath string, partial bool) (string, float64) {
	f, err := os.Open(filePath)
	if err != nil {
		log.Errorf("Failed to open file for hashing: %v", err)
		return "error_hash", 0.0
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		log.Errorf("Failed to stat file: %v", err)
		return "error_hash", 0.0
	}
	fileSize := info.Size()

	hasher := blake3.New()

	if partial {
		const chunkSize = 16 * 1024 * 1024 // 16MB

		// Start
		buf := make([]byte, chunkSize)
		n, _ := io.ReadFull(f, buf)
		hasher.Write(buf[:n])

		// Middle
		if fileSize > chunkSize*2 {
			middle := fileSize / 2
			f.Seek(middle, 0)
			n, _ = io.ReadFull(f, buf)
			hasher.Write(buf[:n])
		}

		// End
		if fileSize > chunkSize {
			f.Seek(-chunkSize, 2)
			n, _ = io.ReadFull(f, buf)
			hasher.Write(buf[:n])
		}

		// Mix file size
		sizeBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(sizeBuf, uint64(fileSize))
		hasher.Write(sizeBuf)
	} else {
		buf := make([]byte, 128*1024)
		if _, err := io.CopyBuffer(hasher, f, buf); err != nil {
			log.Errorf("Failed to hash file: %v", err)
			return "error_hash", 0.0
		}
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), 0.0
}

func getDriveRoot(filePath string) string {
	vol := filepath.VolumeName(filePath)
	if vol == "" {
		return "/"
	}
	return vol + "\\"
}

func getParentFolderName(filePath, driveRoot string) string {
	dir := filepath.Dir(filePath)
	if dir == driveRoot || dir == strings.TrimSuffix(driveRoot, "\\") {
		return "ROOT"
	}
	return filepath.Base(dir)
}

// getRelativePath returns the relative path from driveRoot to the file's directory
func getRelativePath(filePath, driveRoot string) string {
	dir := filepath.Dir(filePath)

	// Normalize paths
	driveRoot = filepath.Clean(driveRoot)
	dir = filepath.Clean(dir)

	// If file is directly in drive root
	if dir == driveRoot {
		return "ROOT"
	}

	// Get relative path from drive root to the file's directory
	relPath, err := filepath.Rel(driveRoot, dir)
	if err != nil {
		// Fallback to just parent folder name
		return filepath.Base(dir)
	}

	// Clean up any .. or . in the path
	relPath = filepath.Clean(relPath)

	return relPath
}

func sanitizeFolderName(name string) string {
	// For paths with multiple levels, sanitize each segment
	// but keep the path separators
	parts := strings.Split(name, string(filepath.Separator))
	for i, part := range parts {
		invalid := []string{":", "*", "?", "\"", "<", ">", "|"}
		result := part
		for _, char := range invalid {
			result = strings.ReplaceAll(result, char, "_")
		}
		parts[i] = result
	}
	return filepath.Join(parts...)
}

func buildStatsSummary(elapsed time.Duration) []string {
	stats.Mu.Lock()
	defer stats.Mu.Unlock()

	var lines []string
	lines = append(lines,
		"",
		"╔════════════════════════════════════════════════════════════════╗",
		"║           VIDEO CONVERSION SUMMARY                             ║",
		"╠════════════════════════════════════════════════════════════════╣",
		fmt.Sprintf("║ Files Analyzed:     %-43d║", stats.FilesAnalyzed),
		fmt.Sprintf("║ Files Converted:    %-43d║", stats.FilesProcessed),
		fmt.Sprintf("║   → Improved:       %-43d║", stats.FilesImproved),
		fmt.Sprintf("║   → Discarded:      %-43d║", stats.FilesDiscarded),
		fmt.Sprintf("║ Files Skipped:      %-43d║", stats.FilesSkipped),
		fmt.Sprintf("║ Files Errored:      %-43d║", stats.FilesErrored),
		"╠════════════════════════════════════════════════════════════════╣",
	)

	if stats.OriginalBytes > 0 {
		saved := stats.OriginalBytes - stats.FinalBytes
		pct := float64(saved) / float64(stats.OriginalBytes) * 100

		origStr := formatBytes(stats.OriginalBytes)
		finalStr := formatBytes(stats.FinalBytes)
		savedStr := formatBytes(saved)

		lines = append(lines,
			fmt.Sprintf("║ Original Size:      %-43s║", origStr),
			fmt.Sprintf("║ Final Size:         %-43s║", finalStr),
			fmt.Sprintf("║ Space Saved:        %-33s (%.1f%%)║", savedStr, pct),
		)
	}

	if elapsed > 0 {
		lines = append(lines,
			"╠════════════════════════════════════════════════════════════════╣",
			fmt.Sprintf("║ Total Time:         %-43s║", fmtElapsed(elapsed)),
		)
	}

	lines = append(lines,
		"╚════════════════════════════════════════════════════════════════╝",
		"",
	)
	return lines
}

func fmtElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func openHSortedFolders() {
	stats.Mu.Lock()
	drives := make([]string, 0, len(stats.TouchedDrives))
	for drive := range stats.TouchedDrives {
		drives = append(drives, drive)
	}
	stats.Mu.Unlock()

	for _, drive := range drives {
		hsortedPath := filepath.Join(drive, "HSORTED")
		if _, err := os.Stat(hsortedPath); err == nil {
			cmd := exec.Command("cmd", "/c", "start", "", hsortedPath)
			cmd.Start()
		}
	}
}

// jobStringer wraps a types.Job to implement fmt.Stringer for FallbackManager.
type jobStringer struct {
	job types.Job
}

func (js *jobStringer) String() string {
	return filepath.Base(js.job.FilePath)
}

// buildConversionArgs constructs FFmpeg arguments using encoder-specific quality and device args.
func buildConversionArgs(inputPath, outputPath, outputExt, encoderName string, qualityArgs, deviceArgs []string) []string {
	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-progress", "pipe:1",
		"-i", inputPath,
	}

	// Add device selection args (e.g., -gpu 0 for NVENC)
	if len(deviceArgs) > 0 {
		args = append(args, deviceArgs...)
	}

	args = append(args, "-c:v", encoderName)
	args = append(args, qualityArgs...)

	if outputExt == ".mkv" {
		args = append(args,
			"-c:a", "copy",
			"-c:s", "copy",
			"-map", "0",
			"-map_metadata", "0",
			"-map_chapters", "0",
		)
	} else {
		args = append(args,
			"-c:a", "aac",
			"-movflags", "+faststart",
		)
	}

	args = append(args, outputPath)
	return args
}
