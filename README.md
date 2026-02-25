# Video Converter

A high-performance CLI batch video conversion tool for Windows that converts videos to HEVC/H.265 format with intelligent caching, concurrent processing, and detailed conversion reporting.

## ✨ Key Features

- **🎬 Batch Conversion** - Process entire directories of videos automatically
- **⚡ HEVC/H.265 Encoding** - Significant file size reduction with maintained quality
- **🔄 Smart Caching** - Per-drive BLAKE3 hash-based cache prevents re-processing
- **🚀 Concurrent Processing** - Producer-consumer pipeline for efficient batch operations
- **📊 Detailed Reporting** - Per-file conversion statistics with size comparisons
- **🎯 Quality Presets** - Configurable quality settings (high_quality, balanced, space_saver)
- **📁 Directory Preservation** - Maintains folder structure from dropped directory onwards
- **🧹 Smart Output** - Only creates directories when conversions succeed
- **🎮 Multiple Encoders** - Support for libx265, nvenc_hevc, and more
- **📝 Optional Seq Logging** - Integration with Seq for centralized logging
- **🖥️ Windows Optimized** - Auto-opens output folders after completion

## 🚀 Quick Start

1. **Drop a folder** on `video-converter.exe` or run from command line:
   ```bash
   video-converter.exe D:\Videos\
   ```

2. **Answer the prompts**:
   - Force re-conversion? [y/N]
   - Test HEVC files too? [y/N]
   - Output drive (optional)

3. **Watch it work**:
   ```
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   Converting [3/25]: video.mp4 (1080p)
   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   Progress: 10%
   Progress: 20%
   ...
   ┌────────────────────────────────────────────────────────────────┐
   │ File: video.mp4                                                │
   ├────────────────────────────────────────────────────────────────┤
   │ Original Size:    284.66 MB                                    │
   │ New Size:         198.45 MB                                    │
   │ Saved:             86.21 MB (30.3% reduction)                  │
   ├────────────────────────────────────────────────────────────────┤
   │ Result: ✓ KEPT - File will be saved                           │
   └────────────────────────────────────────────────────────────────┘
   ```

4. **Find converted videos** at `D:\HSORTED\<your-folder>\`

## 📋 Requirements

- **Windows** (tested on Windows 10/11)
- **FFmpeg** - Place in `ffmpeg\bin\` relative to exe, or configure path
- **MediaInfo CLI** - Place in `MediaInfo_CLI_24.04_Windows_x64\` or configure path
- **NVIDIA GPU** (optional) - For hardware-accelerated encoding with `hevc_nvenc`

## 📦 Installation

### Option 1: Download Binary (Recommended)
1. Download `video-converter.exe` from releases
2. Download [FFmpeg](https://ffmpeg.org/download.html) and extract to `ffmpeg\bin\` folder
3. Download [MediaInfo CLI](https://mediaarea.net/en/MediaInfo/Download/Windows) and extract
4. Run `video-converter.exe` - config will be auto-created on first run

### Option 2: Build from Source
```bash
# Clone the repository
git clone <your-repo-url>
cd VideoConverter

# Download dependencies
go mod download

# Build
go build -o video-converter.exe

# Or optimized build
go build -ldflags="-s -w" -o video-converter.exe
```

## 🎯 Usage

### Basic Usage
```bash
# Convert all videos in a directory
video-converter.exe D:\Videos\

# Dry run (preview without converting)
video-converter.exe --dry-run D:\Videos\

# Use custom config
video-converter.exe --config myconfig.json D:\Videos\

# Override encoder
video-converter.exe --encoder libx265 D:\Videos\
```

### Interactive Prompts
When you run the tool, it will ask:

1. **Force re-conversion (bypass DB check)?** [y/N]
   - `N` (default): Skip files already processed (cached)
   - `Y`: Re-process everything, ignoring cache

2. **Test re-compression even if file is already HEVC?** [y/N]
   - `N` (default): Skip files already in HEVC format
   - `Y`: Re-encode HEVC files anyway

3. **Output drive** (optional):
   - Press Enter: Output to same drive as source (`D:\HSORTED\`)
   - Specify drive: Output to different drive (`E:\` → `E:\HSORTED\`)

## ⚙️ Configuration

The tool auto-creates `configVideoConversion.json` on first run. Key settings:

```json
{
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced",
  "max_queue_size": 3,
  "use_partial_hash": true
}
```

### Quality Presets

| Preset | Use When | Size Reduction | Quality |
|--------|----------|----------------|---------|
| `high_quality` | Archival, max quality | May increase | Excellent |
| `balanced` | General use (default) | 20-40% | Very Good |
| `space_saver` | Already compressed videos | 40-60% | Good |

**Tip:** If files are getting larger, change to `"space_saver"` preset.

See [QUALITY_SETTINGS.md](QUALITY_SETTINGS.md) for detailed quality configuration.

## 📁 Output Structure

The tool preserves directory structure **from the dropped folder onwards**:

**Example:**
```
Input:  Drop C:\Temp\Movies on the exe
        File: C:\Temp\Movies\Action\2024\video.mkv

Output: D:\HSORTED\Movies\Action\2024\video.mp4
```

**Key behaviors:**
- Base directory name is preserved (`Movies` in example above)
- Subdirectory structure is maintained
- **Directories only created if conversion succeeds** (no empty folders)
- Failed/discarded conversions don't create output directories

### Output Formats
- **MP4** → `.mp4` (default for most formats)
- **MKV** → `.mkv` (preserves all audio/subtitle streams & metadata)

## 📚 Documentation

- **[QUICK_START.md](QUICK_START.md)** - Getting started guide with tips
- **[QUALITY_SETTINGS.md](QUALITY_SETTINGS.md)** - Complete quality configuration guide
- **[CHANGELOG.md](CHANGELOG.md)** - Version history and improvements
- **[AGENTS.md](AGENTS.md)** - Developer guide and coding standards

## 💡 Examples

### Scenario 1: Space-Saving Conversion
You have a collection of videos taking up too much space:

```bash
video-converter.exe E:\Movies\
# Choose preset "space_saver" in config for maximum compression
# Expected: 40-60% size reduction
```

### Scenario 2: Archive Quality Conversion
Converting high-quality source files for archival:

```bash
video-converter.exe --encoder libx265 D:\Archive\
# Use preset "high_quality" in config
# Prioritizes quality over file size
```

### Scenario 3: Re-encode Everything
Force re-conversion of all files including cached and HEVC:

```bash
video-converter.exe D:\Videos\
# Answer "y" to both prompts
# Bypasses cache and re-encodes HEVC files
```

### Scenario 4: Output to Different Drive
Convert videos but save to a different drive:

```bash
video-converter.exe C:\Videos\
# When prompted for output drive, enter: E:\
# Output will be: E:\HSORTED\Videos\...
```

## 🔧 Troubleshooting

### Files are getting LARGER after conversion
**Problem:** Quality settings too high for already-compressed videos.

**Solution:**
```json
{
  "quality_preset": "space_saver"
}
```

### All conversions are discarded
**Problem:** Videos may already be optimally compressed.

**Solution:** 
- This is normal - original files are already efficient
- Use `--force-hevc` flag to re-encode anyway (may not improve size)

### FFmpeg not found
**Problem:** FFmpeg executable not in expected location.

**Solution:**
```json
{
  "ffmpeg_path": "C:\\path\\to\\ffmpeg.exe"
}
```

### MediaInfo errors
**Problem:** MediaInfo CLI not found or incorrect path.

**Solution:**
```json
{
  "mediainfo_path": "C:\\path\\to\\MediaInfo.exe"
}
```

### NVIDIA encoding errors
**Problem:** GPU encoding fails or not available.

**Solution:** Switch to CPU encoding:
```json
{
  "video_encoder": "libx265"
}
```

## 📊 Conversion Statistics

After processing, you'll see a comprehensive summary:

```
╔════════════════════════════════════════════════════════════════╗
║           VIDEO CONVERSION SUMMARY                             ║
╠════════════════════════════════════════════════════════════════╣
║ Files Analyzed:     25                                         ║
║ Files Converted:    12                                         ║
║   → Improved:       8                                          ║
║   → Discarded:      4                                          ║
║ Files Skipped:      11                                         ║
║ Files Errored:      2                                          ║
╠════════════════════════════════════════════════════════════════╣
║ Original Size:      2.5 GB                                     ║
║ Final Size:         1.8 GB                                     ║
║ Space Saved:        700.0 MB (28.0%)                           ║
╚════════════════════════════════════════════════════════════════╝
```

## 🏗️ Architecture

- **Producer-Consumer Pipeline** - Concurrent file processing with configurable queue size
- **BLAKE3 Hashing** - Fast file identification (partial or full hash)
- **Per-Drive Caching** - JSON database at drive root (`D:\converted_files.json`)
- **Atomic File Operations** - Safe writes with temp files and rename
- **Thread-Safe Database** - RWMutex for concurrent access
- **Smart Progress Reporting** - FFmpeg progress parsing via stdout

## 🤝 Contributing

This is a personal project, but suggestions and bug reports are welcome! Please check existing documentation before opening issues.

## 📝 License

[Specify your license here - e.g., MIT, GPL-3.0, etc.]

## 🙏 Acknowledgments

- Built with [Go](https://golang.org/)
- Uses [FFmpeg](https://ffmpeg.org/) for video encoding
- Uses [MediaInfo](https://mediaarea.net/) for video analysis
- BLAKE3 hashing via [zeebo/blake3](https://github.com/zeebo/blake3)
- Logging via [logrus](https://github.com/sirupsen/logrus)

---

**Made with ❤️ for efficient video storage**
