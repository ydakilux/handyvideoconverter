package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

var log = logrus.New()

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

// Config holds application configuration
type Config struct {
	ServerURL          string   `json:"server_url"`
	APIKey             string   `json:"api_key"`
	UsePartialHash     bool     `json:"use_partial_hash"`
	MaxQueueSize       int      `json:"max_queue_size"`
	MediaInfoPath      string   `json:"mediainfo_path"`
	FFmpegPath         string   `json:"ffmpeg_path"`
	FFprobePath        string   `json:"ffprobe_path"`
	TempDirectory      string   `json:"temp_directory"`
	VideoEncoder       string   `json:"video_encoder"`
	QualityPreset      string   `json:"quality_preset"`
	CustomQualitySD    int      `json:"custom_quality_sd,omitempty"`
	CustomQuality720p  int      `json:"custom_quality_720p,omitempty"`
	CustomQuality1080p int      `json:"custom_quality_1080p,omitempty"`
	CustomQuality4K    int      `json:"custom_quality_4k,omitempty"`
	FileExtensions     []string `json:"file_extensions"`
	LogLevel           string   `json:"log_level"`
}

// Record represents a cache entry
type Record struct {
	OriginalSize  int64  `json:"original_size"`
	ConvertedSize int64  `json:"converted_size,omitempty"`
	Output        string `json:"output,omitempty"`
	Note          string `json:"note,omitempty"`
	Error         string `json:"error,omitempty"`
}

// DatabaseManager manages per-drive cache files
type DatabaseManager struct {
	mu    sync.RWMutex
	dbs   map[string]map[string]Record // drive -> hash -> record
	dirty map[string]bool
}

// Job represents a conversion job
type Job struct {
	FilePath        string
	BaseDir         string // User-provided input directory (dropped folder)
	DriveRoot       string
	FileHash        string
	OriginalSize    int64
	Width           int
	Height          int
	Format          string
	CodecID         string
	DurationSeconds float64
	FileNumber      int // Current file number
	TotalFiles      int // Total files to process
	FolderNumber    int // Current folder number
	TotalFolders    int // Total folders
}

// Stats tracks conversion statistics
type Stats struct {
	mu             sync.Mutex
	FilesAnalyzed  int
	FilesProcessed int
	FilesImproved  int
	FilesDiscarded int
	FilesSkipped   int
	FilesErrored   int
	OriginalBytes  int64
	FinalBytes     int64
	TouchedDrives  map[string]bool
}

var (
	config              Config
	dbManager           *DatabaseManager
	stats               Stats
	execDir             string
	outputDriveOverride string // If set, use this drive instead of source drive
)

func main() {
	// Parse flags
	var (
		configFile  = flag.String("config", "configVideoConversion.json", "Path to config file")
		dryRun      = flag.Bool("dry-run", false, "Dry run mode")
		bypass      bool
		forceHevc   bool
		encoderFlag = flag.String("encoder", "", "Override video encoder")
	)
	flag.Parse()

	paths := flag.Args()

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		os.Exit(1)
	}
	execDir = filepath.Dir(exePath)

	// Load or create config
	configPath := *configFile
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(execDir, configPath)
	}
	if err := loadConfig(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	setupLogging()

	// Override encoder if specified
	if *encoderFlag != "" {
		config.VideoEncoder = *encoderFlag
	}

	// Interactive prompts if not specified
	if !contains(os.Args, "--bypass") {
		bypass = askYesNo("Force re-conversion (bypass DB check)? [y/N]: ")
	}
	if !contains(os.Args, "--force-hevc") {
		forceHevc = askYesNo("Test re-compression even if file is already HEVC? [y/N]: ")
	}

	// Ask about output drive
	outputDriveOverride = askOutputDrive()

	// Validate paths
	if len(paths) == 0 {
		log.Fatal("No input paths specified")
	}

	// Initialize database manager
	dbManager = &DatabaseManager{
		dbs:   make(map[string]map[string]Record),
		dirty: make(map[string]bool),
	}

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
	jobQueue := make(chan Job, config.MaxQueueSize)
	var wg sync.WaitGroup

	// Start consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer(jobQueue, *dryRun)
	}()

	// Start producer
	producer(files, fileToBaseDir, jobQueue, bypass, forceHevc)

	// Wait for consumer to finish
	wg.Wait()

	// Save all databases
	dbManager.saveAll()

	// Print stats
	printStats()

	// Auto-open HSORTED folders
	openHSortedFolders()

	log.Info("All tasks completed")
	log.Info("")
	log.Info("Press Enter to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func loadConfig(path string) error {
	// Check if config exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create default config
		defaultConfig := Config{
			ServerURL:          "http://localhost:5341/",
			APIKey:             "",
			UsePartialHash:     true,
			MaxQueueSize:       3,
			MediaInfoPath:      "MediaInfo_CLI_24.04_Windows_x64\\MediaInfo.exe",
			FFmpegPath:         "ffmpeg\\bin\\ffmpeg.exe",
			FFprobePath:        "",
			TempDirectory:      "",
			VideoEncoder:       "hevc_nvenc",
			QualityPreset:      "balanced",
			CustomQualitySD:    0,
			CustomQuality720p:  0,
			CustomQuality1080p: 0,
			CustomQuality4K:    0,
			FileExtensions:     []string{".MOV", ".AVI", ".MKV", ".MP4", ".WMV", ".M4V", ".FLV", ".MPG", ".ASF", ".TS", ".M2TS"},
			LogLevel:           "INFO",
		}
		data, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
		config = defaultConfig
		return nil
	}

	// Load existing config
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &config)
}

func setupLogging() {
	// Create log file with timestamp
	logFileName := fmt.Sprintf("video-converter_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logFilePath := filepath.Join(execDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
	} else {
		// Write to both console and file
		// Console: simple format without timestamps
		// File: full format with timestamps and levels
		mw := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(mw)
		fmt.Printf("Logging to: %s\n", logFilePath)
	}

	// Custom formatter that only outputs the message (for console)
	log.SetFormatter(&SimpleFormatter{})

	// Set log level
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	// Add Seq hook if configured (Seq will get full timestamps and levels)
	if config.ServerURL != "" {
		hook := NewSeqHook(config.ServerURL, config.APIKey)
		log.AddHook(hook)
		log.Debug("Seq logging hook enabled")
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

func producer(files []string, fileToBaseDir map[string]string, jobQueue chan<- Job, bypass, forceHevc bool) {
	defer close(jobQueue)

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

			stats.mu.Lock()
			stats.FilesAnalyzed++
			stats.mu.Unlock()

			// Get file info
			info, err := os.Stat(filePath)
			if err != nil {
				log.Warnf("Failed to stat %s: %v", filePath, err)
				stats.mu.Lock()
				stats.FilesErrored++
				stats.mu.Unlock()
				continue
			}

			// Get drive root
			driveRoot := getDriveRoot(filePath)
			stats.mu.Lock()
			stats.TouchedDrives[driveRoot] = true
			stats.mu.Unlock()

			// Compute hash
			fileHash, _ := getFileHash(filePath, config.UsePartialHash)
			if fileHash == "error_hash" {
				log.Warnf("Failed to hash %s", filePath)
				stats.mu.Lock()
				stats.FilesErrored++
				stats.mu.Unlock()
				continue
			}

			// Check cache
			if !bypass {
				rec := dbManager.getRecord(driveRoot, fileHash)
				if rec != nil && (rec.Output != "" || rec.Note == "not_beneficial" || rec.Note == "already_hevc") {
					log.Debugf("Skipping %s (already processed)", filePath)
					stats.mu.Lock()
					stats.FilesSkipped++
					stats.mu.Unlock()
					continue
				}
			}

			// Get MediaInfo
			videoInfo, err := getMediaInfo(filePath)
			if err != nil {
				log.Warnf("Failed to get MediaInfo for %s: %v", filePath, err)
				stats.mu.Lock()
				stats.FilesErrored++
				stats.mu.Unlock()
				continue
			}
			if videoInfo == nil {
				log.Warnf("No video track found in %s", filePath)
				stats.mu.Lock()
				stats.FilesErrored++
				stats.mu.Unlock()
				continue
			}

			// Check if already HEVC
			if isHEVC(videoInfo.Format, videoInfo.CodecID) && !forceHevc {
				log.Infof("Skipping %s (already HEVC)", filePath)
				dbManager.updateRecord(driveRoot, fileHash, Record{
					OriginalSize:  info.Size(),
					ConvertedSize: info.Size(),
					Note:          "already_hevc",
				})
				stats.mu.Lock()
				stats.FilesSkipped++
				stats.mu.Unlock()
				continue
			}

			// Get duration via FFprobe
			duration := getDuration(filePath)

			// Enqueue job
			job := Job{
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
			jobQueue <- job
		}
	}
}

func consumer(jobQueue <-chan Job, dryRun bool) {
	for job := range jobQueue {
		processConversion(job, dryRun)
		dbManager.saveAll()
	}
}

func processConversion(job Job, dryRun bool) {
	fileName := filepath.Base(job.FilePath)

	log.Info("")
	log.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Infof("Converting [%d/%d]: %s (%dp)", job.FileNumber, job.TotalFiles, fileName, job.Height)
	if strings.ToLower(filepath.Ext(job.FilePath)) == ".mkv" {
		log.Infof("Format: MKV (preserving all audio/subtitle streams & metadata)")
	}
	log.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if dryRun {
		log.Infof("[DRY RUN] Would convert: %s (%dp)", fileName, job.Height)
		return
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

	// Build FFmpeg command
	quality := determineQuality(job.Width)
	args := buildFFmpegArgs(job.FilePath, tempPath, quality, outputExt)

	// Run FFmpeg
	ffmpegExe := resolveExecutable(config.FFmpegPath, "ffmpeg.exe")
	rc := runFFmpeg(ffmpegExe, args, job.FilePath, job.DurationSeconds)

	if rc != 0 {
		log.Errorf("FFmpeg failed with exit code %d for %s", rc, job.FilePath)
		dbManager.updateRecord(job.DriveRoot, job.FileHash, Record{
			OriginalSize: job.OriginalSize,
			Error:        fmt.Sprintf("rc_%d", rc),
		})
		os.Remove(tempPath)
		stats.mu.Lock()
		stats.FilesErrored++
		stats.mu.Unlock()
		return
	}

	// Check output size
	tempInfo, err := os.Stat(tempPath)
	if err != nil {
		log.Errorf("Failed to stat temp file: %v", err)
		os.Remove(tempPath)
		return
	}

	newSize := tempInfo.Size()

	// Print conversion result
	origMB := float64(job.OriginalSize) / (1024 * 1024)
	newMB := float64(newSize) / (1024 * 1024)

	log.Info("┌────────────────────────────────────────────────────────────────┐")
	log.Infof("│ File: %-56s │", truncateString(fileName, 56))
	log.Info("├────────────────────────────────────────────────────────────────┤")
	log.Infof("│ Original Size:  %8.2f MB                                    │", origMB)
	log.Infof("│ New Size:       %8.2f MB                                    │", newMB)

	if newSize < job.OriginalSize {
		// KEPT - File improved
		reduction := float64(job.OriginalSize-newSize) / float64(job.OriginalSize) * 100
		savedMB := origMB - newMB

		log.Infof("│ Saved:          %8.2f MB (%.1f%% reduction)                │", savedMB, reduction)
		log.Info("├────────────────────────────────────────────────────────────────┤")
		log.Info("│ Result: ✓ KEPT - File will be saved                           │")
		log.Info("└────────────────────────────────────────────────────────────────┘")
		log.Info("")

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

		dbManager.updateRecord(job.DriveRoot, job.FileHash, Record{
			OriginalSize:  job.OriginalSize,
			ConvertedSize: newSize,
			Output:        finalPath,
		})

		stats.mu.Lock()
		stats.FilesProcessed++
		stats.FilesImproved++
		stats.OriginalBytes += job.OriginalSize
		stats.FinalBytes += newSize
		stats.mu.Unlock()
	} else {
		// DISCARDED - File not improved
		increase := float64(newSize-job.OriginalSize) / float64(job.OriginalSize) * 100
		increasedMB := newMB - origMB

		log.Infof("│ Increased:      %8.2f MB (+%.1f%%)                         │", increasedMB, increase)
		log.Info("├────────────────────────────────────────────────────────────────┤")
		log.Info("│ Result: ✗ DISCARDED - File not improved, keeping original     │")
		log.Info("└────────────────────────────────────────────────────────────────┘")
		log.Info("")

		os.Remove(tempPath)

		dbManager.updateRecord(job.DriveRoot, job.FileHash, Record{
			OriginalSize:  job.OriginalSize,
			ConvertedSize: newSize,
			Note:          "not_beneficial",
		})

		stats.mu.Lock()
		stats.FilesProcessed++
		stats.FilesDiscarded++
		stats.OriginalBytes += job.OriginalSize
		stats.FinalBytes += job.OriginalSize
		stats.mu.Unlock()
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

type VideoInfo struct {
	Format  string
	Width   int
	Height  int
	CodecID string
}

func getMediaInfo(filePath string) (*VideoInfo, error) {
	mediaInfoExe := resolveExecutable(config.MediaInfoPath, "MediaInfo.exe")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, mediaInfoExe, "--output=JSON", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Media struct {
			Track []map[string]interface{} `json:"track"`
		} `json:"media"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	for _, track := range result.Media.Track {
		if trackType, ok := track["@type"].(string); ok && trackType == "Video" {
			info := &VideoInfo{}

			if format, ok := track["Format"].(string); ok {
				info.Format = format
			}
			if codecID, ok := track["CodecID"].(string); ok {
				info.CodecID = codecID
			}
			if width, ok := track["Width"].(string); ok {
				fmt.Sscanf(width, "%d", &info.Width)
			}
			if height, ok := track["Height"].(string); ok {
				fmt.Sscanf(height, "%d", &info.Height)
			}

			return info, nil
		}
	}

	return nil, fmt.Errorf("no video track found")
}

func getDuration(filePath string) float64 {
	ffprobeExe := resolveExecutable(config.FFprobePath, "ffprobe.exe")
	if ffprobeExe == "" {
		ffmpegExe := resolveExecutable(config.FFmpegPath, "ffmpeg.exe")
		ffprobeExe = filepath.Join(filepath.Dir(ffmpegExe), "ffprobe.exe")
		if _, err := os.Stat(ffprobeExe); err != nil {
			ffprobeExe, _ = exec.LookPath("ffprobe.exe")
		}
	}

	if ffprobeExe == "" {
		return 0.0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobeExe, "-v", "error", "-show_entries", "format=duration", "-of", "json", filePath)
	output, err := cmd.Output()
	if err != nil {
		return 0.0
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return 0.0
	}

	duration, _ := strconv.ParseFloat(result.Format.Duration, 64)
	return duration
}

func isHEVC(format, codecID string) bool {
	return strings.Contains(strings.ToUpper(format), "HEVC") ||
		strings.Contains(strings.ToUpper(codecID), "HVC1")
}

func determineQuality(width int) string {
	// Check if custom quality values are set
	if config.CustomQualitySD > 0 && width <= 1024 {
		return strconv.Itoa(config.CustomQualitySD)
	}
	if config.CustomQuality720p > 0 && width <= 1280 {
		return strconv.Itoa(config.CustomQuality720p)
	}
	if config.CustomQuality1080p > 0 && width <= 1920 {
		return strconv.Itoa(config.CustomQuality1080p)
	}
	if config.CustomQuality4K > 0 && width > 1920 {
		return strconv.Itoa(config.CustomQuality4K)
	}

	// Use preset if no custom values
	switch strings.ToLower(config.QualityPreset) {
	case "high_quality":
		// Prioritize quality over file size (current default)
		if width <= 1024 {
			return "19"
		} else if width <= 1280 {
			return "20"
		} else if width <= 1920 {
			return "21"
		}
		return "23"

	case "balanced":
		// Balance between quality and file size (recommended)
		if width <= 1024 {
			return "23"
		} else if width <= 1280 {
			return "25"
		} else if width <= 1920 {
			return "27"
		}
		return "30"

	case "space_saver":
		// Prioritize file size over quality
		if width <= 1024 {
			return "26"
		} else if width <= 1280 {
			return "28"
		} else if width <= 1920 {
			return "30"
		}
		return "33"

	default:
		// Default to balanced
		if width <= 1024 {
			return "23"
		} else if width <= 1280 {
			return "25"
		} else if width <= 1920 {
			return "27"
		}
		return "30"
	}
}

func buildFFmpegArgs(inputPath, outputPath, quality, outputExt string) []string {
	args := []string{
		"-hide_banner", "-y", "-nostats",
		"-progress", "pipe:1",
		"-i", inputPath,
		"-c:v", config.VideoEncoder,
	}

	// Add quality parameter
	if config.VideoEncoder == "libx265" {
		args = append(args, "-crf", quality, "-preset", "medium")
	} else if strings.Contains(config.VideoEncoder, "_nvenc") {
		args = append(args, "-cq", quality, "-preset", "p5")
	} else {
		args = append(args, "-crf", quality)
	}

	// Handle audio and subtitles based on output format
	if outputExt == ".mkv" {
		// MKV: Copy all audio streams, copy all subtitle streams, copy attachments
		args = append(args,
			"-c:a", "copy", // Copy all audio streams without re-encoding
			"-c:s", "copy", // Copy all subtitle streams
			"-map", "0", // Map all streams from input
			"-map_metadata", "0", // Copy all metadata
			"-map_chapters", "0", // Copy chapters
		)
	} else {
		// MP4: Re-encode audio to AAC, add faststart for streaming
		args = append(args,
			"-c:a", "aac",
			"-movflags", "+faststart",
		)
	}

	args = append(args, outputPath)

	return args
}

func runFFmpeg(ffmpegExe string, args []string, filePath string, totalDuration float64) int {
	cmd := exec.Command(ffmpegExe, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Errorf("Failed to create stdout pipe: %v", err)
		return 999
	}

	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start FFmpeg: %v", err)
		return 999
	}

	lastPct := -1
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_ms=") || strings.HasPrefix(line, "out_time_us=") || strings.HasPrefix(line, "out_time=") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				var outTime float64
				if strings.HasPrefix(line, "out_time_ms=") {
					if val, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						outTime = float64(val) / 1000000.0
					}
				} else if strings.HasPrefix(line, "out_time_us=") {
					if val, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						outTime = float64(val) / 1000000.0
					}
				} else if strings.HasPrefix(line, "out_time=") {
					// Parse HH:MM:SS.mmm
					timeParts := strings.Split(parts[1], ":")
					if len(timeParts) == 3 {
						h, _ := strconv.Atoi(timeParts[0])
						m, _ := strconv.Atoi(timeParts[1])
						s, _ := strconv.ParseFloat(timeParts[2], 64)
						outTime = float64(h*3600+m*60) + s
					}
				}

				if totalDuration > 0 && outTime > 0 {
					pct := int(100 * outTime / totalDuration)
					if pct > lastPct && (pct%10 == 0 || pct == 100) {
						log.Infof("Progress: %d%%", pct)
						lastPct = pct
					}
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 999
	}

	return 0
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

func resolveExecutable(configPath, exeName string) string {
	if configPath == "" {
		exe, _ := exec.LookPath(exeName)
		return exe
	}

	var path string
	if filepath.IsAbs(configPath) {
		path = configPath
	} else {
		path = filepath.Join(execDir, configPath)
	}

	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Try PATH
	exe, _ := exec.LookPath(exeName)
	return exe
}

func (db *DatabaseManager) getRecord(driveRoot, fileHash string) *Record {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.dbs[driveRoot] == nil {
		db.mu.RUnlock()
		db.mu.Lock()
		db.loadDB(driveRoot)
		db.mu.Unlock()
		db.mu.RLock()
	}

	if rec, ok := db.dbs[driveRoot][fileHash]; ok {
		return &rec
	}
	return nil
}

func (db *DatabaseManager) updateRecord(driveRoot, fileHash string, rec Record) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.dbs[driveRoot] == nil {
		db.loadDB(driveRoot)
	}

	db.dbs[driveRoot][fileHash] = rec
	db.dirty[driveRoot] = true
}

func (db *DatabaseManager) loadDB(driveRoot string) {
	dbPath := filepath.Join(driveRoot, "converted_files.json")
	data, err := os.ReadFile(dbPath)
	if err != nil {
		db.dbs[driveRoot] = make(map[string]Record)
		return
	}

	var records map[string]Record
	if err := json.Unmarshal(data, &records); err != nil {
		db.dbs[driveRoot] = make(map[string]Record)
		return
	}

	db.dbs[driveRoot] = records
}

func (db *DatabaseManager) saveAll() {
	db.mu.Lock()
	defer db.mu.Unlock()

	for driveRoot, isDirty := range db.dirty {
		if !isDirty {
			continue
		}

		dbPath := filepath.Join(driveRoot, "converted_files.json")
		tmpPath := dbPath + ".tmp"

		data, err := json.MarshalIndent(db.dbs[driveRoot], "", "  ")
		if err != nil {
			log.Errorf("Failed to marshal DB for %s: %v", driveRoot, err)
			continue
		}

		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			log.Errorf("Failed to write DB temp file for %s: %v", driveRoot, err)
			continue
		}

		if err := os.Rename(tmpPath, dbPath); err != nil {
			log.Errorf("Failed to rename DB file for %s: %v", driveRoot, err)
			continue
		}

		db.dirty[driveRoot] = false
	}
}

func printStats() {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	log.Info("")
	log.Info("╔════════════════════════════════════════════════════════════════╗")
	log.Info("║           VIDEO CONVERSION SUMMARY                             ║")
	log.Info("╠════════════════════════════════════════════════════════════════╣")
	log.Infof("║ Files Analyzed:     %-43d║", stats.FilesAnalyzed)
	log.Infof("║ Files Converted:    %-43d║", stats.FilesProcessed)
	log.Infof("║   → Improved:       %-43d║", stats.FilesImproved)
	log.Infof("║   → Discarded:      %-43d║", stats.FilesDiscarded)
	log.Infof("║ Files Skipped:      %-43d║", stats.FilesSkipped)
	log.Infof("║ Files Errored:      %-43d║", stats.FilesErrored)
	log.Info("╠════════════════════════════════════════════════════════════════╣")

	if stats.OriginalBytes > 0 {
		saved := stats.OriginalBytes - stats.FinalBytes
		pct := float64(saved) / float64(stats.OriginalBytes) * 100

		origStr := formatBytes(stats.OriginalBytes)
		finalStr := formatBytes(stats.FinalBytes)
		savedStr := formatBytes(saved)

		log.Infof("║ Original Size:      %-43s║", origStr)
		log.Infof("║ Final Size:         %-43s║", finalStr)
		log.Infof("║ Space Saved:        %-33s (%.1f%%)║", savedStr, pct)
	}

	log.Info("╚════════════════════════════════════════════════════════════════╝")
	log.Info("")
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
	stats.mu.Lock()
	drives := make([]string, 0, len(stats.TouchedDrives))
	for drive := range stats.TouchedDrives {
		drives = append(drives, drive)
	}
	stats.mu.Unlock()

	for _, drive := range drives {
		hsortedPath := filepath.Join(drive, "HSORTED")
		if _, err := os.Stat(hsortedPath); err == nil {
			cmd := exec.Command("cmd", "/c", "start", "", hsortedPath)
			cmd.Start()
		}
	}
}
