# Video Converter - Quality Settings Guide

## Overview

The video converter now supports flexible quality settings to balance between file size and video quality. You can choose from presets or define custom quality values per resolution.

## Configuration Location

Edit `configVideoConversion.json` (next to the executable)

## Quality Presets

### 1. **high_quality** (Original Default)
Prioritizes quality over file size. May result in larger files.

```json
"quality_preset": "high_quality"
```

**CQ Values:**
- SD (≤1024px): 19
- 720p (≤1280px): 20
- 1080p (≤1920px): 21
- 4K (>1920px): 23

**Use when:** You want maximum quality preservation, even if files get larger.

---

### 2. **balanced** (Recommended - New Default)
Balance between quality and file size reduction.

```json
"quality_preset": "balanced"
```

**CQ Values:**
- SD (≤1024px): 23
- 720p (≤1280px): 25
- 1080p (≤1920px): 27
- 4K (>1920px): 30

**Use when:** You want good quality with noticeable space savings (typical 20-40% reduction).

---

### 3. **space_saver**
Prioritizes file size reduction over quality.

```json
"quality_preset": "space_saver"
```

**CQ Values:**
- SD (≤1024px): 26
- 720p (≤1280px): 28
- 1080p (≤1920px): 30
- 4K (>1920px): 33

**Use when:** Storage space is critical and slight quality loss is acceptable (typical 40-60% reduction).

---

## Custom Quality Values

For fine-tuned control, set custom CQ values per resolution. Custom values override presets.

```json
{
  "quality_preset": "balanced",
  "custom_quality_sd": 24,
  "custom_quality_720p": 26,
  "custom_quality_1080p": 28,
  "custom_quality_4k": 31
}
```

**Set to 0 to use preset value for that resolution.**

---

## Understanding CQ (Constant Quality) Values

For **NVENC (hevc_nvenc)** and **libx265**:
- **Lower values (18-23)** = Higher quality, larger files
- **Medium values (24-28)** = Good quality, moderate file size
- **Higher values (29-35)** = Lower quality, smaller files

### Typical Results by CQ:

| CQ Range | Quality | Typical Size Change | Notes |
|----------|---------|---------------------|-------|
| 18-21 | Excellent | -10% to +20% | May increase size if source is already compressed |
| 22-25 | Very Good | -20% to -40% | Good balance for most content |
| 26-29 | Good | -40% to -60% | Noticeable compression, still acceptable |
| 30-33 | Fair | -60% to -70% | Visible artifacts on detailed scenes |
| 34+ | Poor | -70%+ | Not recommended |

---

## Example Configurations

### For Already-Compressed Videos (like your case)
If files are getting larger, use **space_saver** or increase CQ values:

```json
{
  "quality_preset": "space_saver",
  "video_encoder": "hevc_nvenc"
}
```

Or custom values:
```json
{
  "quality_preset": "balanced",
  "custom_quality_1080p": 30,
  "video_encoder": "hevc_nvenc"
}
```

### For High-Quality Source Files
Use **balanced** for good compression:

```json
{
  "quality_preset": "balanced",
  "video_encoder": "hevc_nvenc"
}
```

### For Archival Quality
Use **high_quality** to preserve maximum detail:

```json
{
  "quality_preset": "high_quality",
  "video_encoder": "hevc_nvenc"
}
```

---

## Testing Your Settings

1. **Backup your config** before making changes
2. **Run with --dry-run** to see what would happen:
   ```bash
   video-converter.exe --dry-run V:\Videos\
   ```
3. **Test on a few files first** before batch processing
4. **Check the results** - files should show reduction, not increase

---

## Troubleshooting

### Files are getting LARGER after conversion

**Problem:** Your quality settings are too high for already-compressed videos.

**Solution:** 
1. Change preset to `"space_saver"`
2. Or increase custom CQ values (28-32 for 1080p)

### Files are too small but quality is poor

**Problem:** Quality settings too aggressive.

**Solution:**
1. Change preset to `"balanced"` or `"high_quality"`
2. Or decrease custom CQ values (23-26 for 1080p)

### Conversions always get discarded

**Problem:** All conversions result in larger files.

**Solution:**
- Source videos may already be HEVC at high compression
- Use `--force-hevc` to re-encode anyway
- Or skip these files (they're already optimal)

---

## Quick Reference

**Default config after update:**
```json
{
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced",
  "custom_quality_sd": 0,
  "custom_quality_720p": 0,
  "custom_quality_1080p": 0,
  "custom_quality_4k": 0
}
```

**To get space savings on your files, change to:**
```json
{
  "video_encoder": "hevc_nvenc",
  "quality_preset": "space_saver"
}
```

Or for fine control:
```json
{
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced",
  "custom_quality_1080p": 30
}
```
