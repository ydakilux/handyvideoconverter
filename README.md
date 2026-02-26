# Video Converter

A high-performance CLI batch video conversion tool for Windows that converts videos to HEVC/H.265 format with GPU auto-detection, intelligent caching, concurrent processing, and detailed conversion reporting.

## ✨ Key Features

- **🎬 Batch Conversion** - Process entire directories of videos automatically
- **⚡ HEVC/H.265 Encoding** - Significant file size reduction with maintained quality
- **🖥️ GPU Auto-Detection** - Automatically finds and uses the best available hardware encoder
- **🔄 Smart Caching** - Per-drive BLAKE3 hash-based cache prevents re-processing
- **🚀 Concurrent Processing** - Producer-consumer pipeline for efficient batch operations
- **📊 Detailed Reporting** - Per-file conversion statistics with size comparisons
- **🎯 Quality Presets** - Configurable quality settings (high_quality, balanced, space_saver)
- **📁 Directory Preservation** - Maintains folder structure from dropped directory onwards
- **🧹 Smart Output** - Only creates directories when conversions succeed
- **🎮 Multiple Encoders** - NVIDIA NVENC, AMD AMF, Intel QSV, and CPU libx265
- **📝 Optional Seq Logging** - Integration with Seq for centralized logging
- **🖥️ Windows Optimized** - Auto-opens output folders after completion

## 🖥️ GPU Auto-Detection

The tool automatically detects your GPU and picks the fastest available encoder:

1. **Probes hardware** at startup for NVIDIA NVENC, AMD AMF, and Intel QSV support
2. **Trial-encodes a short clip** to verify the encoder actually works (not just reported as available)
3. **Benchmarks GPU speed** and caches results so future runs start instantly
4. **Falls back to CPU** (`libx265`) if no working GPU encoder is found

You don't need to configure anything. Just run the tool and it picks the best option. If you want a specific encoder, use `--encoder` to override.

### Supported Encoders

| Encoder | Hardware | Quality Flag | Speed | Notes |
|---------|----------|-------------|-------|-------|
| `auto` | (detect) | (varies) | Best available | **Default.** Picks the fastest working encoder |
| `hevc_nvenc` | NVIDIA GPU | `-cq` | Fastest | Requires NVIDIA drivers with NVENC support |
| `hevc_amf` | AMD GPU | `-qp_i`/`-qp_p` | Fast | Requires AMD drivers with AMF support |
| `hevc_qsv` | Intel GPU | `-global_quality` | Fast | Requires Intel GPU with Quick Sync |
| `libx265` | CPU | `-crf` | Slowest | Always available, most compatible |

### Multi-GPU (NVIDIA)

When multiple NVIDIA GPUs are detected, the tool distributes encoding across them based on benchmark speed. Each GPU gets work proportional to its throughput. AMD and Intel don't support device selection through FFmpeg, so multi-GPU only applies to NVIDIA setups.

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
- **GPU** (optional) - NVIDIA, AMD, or Intel GPU for hardware-accelerated encoding. Falls back to CPU automatically if no GPU is available.

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

### Command Line Flags

```
video-converter.exe [flags] <directory>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `configVideoConversion.json` | Path to config file |
| `--dry-run` | `false` | Preview mode, no actual conversion |
| `--encoder` | `auto` | Encoder selection: `auto`, `hevc_nvenc`, `hevc_amf`, `hevc_qsv`, `libx265` |
| `--non-interactive` | `false` | Disable interactive prompts (auto-fallback to CPU on GPU failure) |
| `--rebenchmark` | `false` | Force GPU benchmark even if cached results exist |

### Basic Usage
```bash
# Convert all videos in a directory (auto-detects best encoder)
video-converter.exe D:\Videos\

# Dry run (preview without converting)
video-converter.exe --dry-run D:\Videos\

# Use custom config
video-converter.exe --config myconfig.json D:\Videos\

# Force a specific encoder
video-converter.exe --encoder libx265 D:\Videos\

# Non-interactive mode (good for scripts/scheduled tasks)
video-converter.exe --non-interactive D:\Videos\

# Re-run GPU benchmark
video-converter.exe --rebenchmark D:\Videos\
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

When a GPU encoder fails mid-conversion, the tool asks whether to retry with CPU encoding. Use `--non-interactive` to skip this prompt and auto-fallback.

## ⚙️ Configuration

The tool auto-creates `configVideoConversion.json` on first run. Key settings:

```json
{
  "video_encoder": "auto",
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

See [QUALITY_SETTINGS.md](QUALITY_SETTINGS.md) for detailed quality configuration, including per-encoder quality parameters.

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

### Scenario 5: Automated/Scripted Conversion
Run from a script without any interactive prompts:

```bash
video-converter.exe --non-interactive --encoder auto D:\Videos\
# GPU auto-detection with automatic CPU fallback on failure
# No user interaction needed
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
- This is normal. Original files are already efficient.
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

### GPU encoding fails
**Problem:** GPU encoder not available or errors during encoding.

**Solution:** The tool auto-falls back to CPU when `--encoder auto` is set. To force CPU encoding:
```bash
video-converter.exe --encoder libx265 D:\Videos\
```
Or in config:
```json
{
  "video_encoder": "libx265"
}
```

### GPU benchmark is slow or stale
**Problem:** First run takes extra time for benchmarking, or cached benchmark results are outdated.

**Solution:** Benchmark results are cached after the first run, so subsequent launches are fast. To force a fresh benchmark:
```bash
video-converter.exe --rebenchmark D:\Videos\
```

### Multi-GPU not distributing work
**Problem:** Only one GPU is being used.

**Solution:** Multi-GPU distribution only works with NVIDIA GPUs. AMD and Intel encoders don't support device selection through FFmpeg. Make sure you have multiple NVIDIA GPUs with up-to-date drivers.

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

- **GPU Auto-Detection** - Trial-encode verification ensures encoder actually works before use
- **Encoder-Specific Quality** - Each encoder gets its own quality normalization (CRF, CQ, QP, global_quality)
- **Fallback Manager** - GPU error recovery with interactive or automatic CPU fallback
- **Producer-Consumer Pipeline** - Concurrent file processing with configurable queue size
- **BLAKE3 Hashing** - Fast file identification (partial or full hash)
- **Per-Drive Caching** - JSON database at drive root (`D:\converted_files.json`)
- **Atomic File Operations** - Safe writes with temp files and rename
- **Thread-Safe Database** - RWMutex for concurrent access
- **Smart Progress Reporting** - FFmpeg progress parsing via stdout
- **Multi-GPU Distribution** - Speed-balanced work distribution across NVIDIA GPUs

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
