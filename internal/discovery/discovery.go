// Package discovery walks input paths and produces conversion Jobs into the pipeline.
package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	cfgpkg "video-converter/internal/config"
	"video-converter/internal/database"
	"video-converter/internal/ffmpeg"
	"video-converter/internal/fileutil"
	"video-converter/internal/pipeline"
	"video-converter/internal/types"
)

// DiscoverFiles walks paths and returns all matching video files with their
// base directories. The base directory is the user-provided input path.
func DiscoverFiles(paths []string, extensions []string, log *logrus.Logger) ([]string, map[string]string) {
	var files []string
	fileToBaseDir := make(map[string]string)
	extMap := make(map[string]bool)
	for _, ext := range extensions {
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
			filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
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
			if extMap[strings.ToLower(filepath.Ext(absPath))] {
				files = append(files, absPath)
				fileToBaseDir[absPath] = filepath.Dir(absPath)
			}
		}
	}

	return files, fileToBaseDir
}

// ProducerConfig holds everything the producer needs to analyse files and
// enqueue jobs — all values are read-only during the run.
type ProducerConfig struct {
	Config      *types.Config
	ExecDir     string
	DBManager   *database.DatabaseManager
	Stats       *types.Stats
	FFprobePath string // resolved ffprobe path (may be empty → resolves from ffmpeg dir)
	Log         *logrus.Logger
}

// Produce analyses files, filters already-processed ones, and submits Jobs to
// the pipeline. It calls pipe.Wait() when done. Run in a goroutine.
func Produce(files []string, fileToBaseDir map[string]string, pipe *pipeline.Pipeline, bypass, forceHevc bool, cfg ProducerConfig) {
	defer pipe.Wait()

	ffprobeExe := resolveFFprobeExe(cfg.Config, cfg.ExecDir)

	// Group files by parent folder for nicer log ordering
	folderMap := make(map[string][]string)
	for _, filePath := range files {
		driveRoot := fileutil.GetDriveRoot(filePath)
		parentFolder := fileutil.GetParentFolderName(filePath, driveRoot)
		folderMap[parentFolder] = append(folderMap[parentFolder], filePath)
	}

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

		cfg.Log.Infof("\nProcessing folder %d/%d: %s (%d files)", folderNumber, totalFolders, folder, len(filesInFolder))

		for _, filePath := range filesInFolder {
			globalFileNumber++
			cfg.Log.Debugf("[%d/%d] Analyzing: %s", globalFileNumber, totalFiles, filepath.Base(filePath))

			cfg.Stats.IncrFilesAnalyzed()

			info, err := os.Stat(filePath)
			if err != nil {
				cfg.Log.Warnf("Failed to stat %s: %v", filePath, err)
				cfg.Stats.IncrFilesErrored()
				continue
			}

			driveRoot := fileutil.GetDriveRoot(filePath)
			cfg.Stats.AddTouchedDrive(driveRoot)

			fileHash, _ := fileutil.GetFileHash(filePath, cfg.Config.UsePartialHash)
			if fileHash == "error_hash" {
				cfg.Log.Warnf("Failed to hash %s", filePath)
				cfg.Stats.IncrFilesErrored()
				continue
			}

			if !bypass {
				rec := cfg.DBManager.GetRecord(driveRoot, fileHash)
				if rec != nil && (rec.Output != "" || rec.Note == "not_beneficial" || rec.Note == "already_hevc") {
					cfg.Log.Debugf("Skipping %s (already processed)", filePath)
					cfg.Stats.IncrFilesSkipped()
					continue
				}
			}

			videoInfo, err := ffmpeg.GetMediaInfo(filePath, ffprobeExe)
			if err != nil {
				cfg.Log.Warnf("Failed to get video info for %s: %v", filePath, err)
				cfg.Stats.IncrFilesErrored()
				continue
			}
			if videoInfo == nil {
				cfg.Log.Warnf("No video track found in %s", filePath)
				cfg.Stats.IncrFilesErrored()
				continue
			}

			if ffmpeg.IsHEVC(videoInfo.Format, videoInfo.CodecID) && !forceHevc {
				cfg.Log.Infof("Skipping %s (already HEVC)", filePath)
				cfg.DBManager.UpdateRecord(driveRoot, fileHash, types.Record{
					OriginalSize:  info.Size(),
					ConvertedSize: info.Size(),
					Note:          "already_hevc",
				})
				cfg.Stats.IncrFilesSkipped()
				continue
			}

			duration := ffmpeg.GetDuration(filePath, ffprobeExe, cfg.Log)

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
				VideoInfo:       videoInfo,
			}
			pipe.Submit(job) //nolint:errcheck
		}
	}
}

// resolveFFprobeExe finds the ffprobe executable from config or by locating it
// beside ffmpeg.
func resolveFFprobeExe(cfg *types.Config, execDir string) string {
	ffprobeExe := cfgpkg.ResolveExecutable(cfg.FFprobePath, "ffprobe.exe", execDir)
	if ffprobeExe == "" {
		ffmpegExe := cfgpkg.ResolveExecutable(cfg.FFmpegPath, "ffmpeg.exe", execDir)
		ffprobeExe = filepath.Join(filepath.Dir(ffmpegExe), "ffprobe.exe")
		if _, err := os.Stat(ffprobeExe); err != nil {
			ffprobeExe, _ = exec.LookPath("ffprobe.exe")
		}
	}
	return ffprobeExe
}
