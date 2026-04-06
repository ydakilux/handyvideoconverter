# Video Converter - Quality Settings Guide

## Overview

The video converter supports flexible quality settings to balance between file size and video quality. Each encoder uses its own quality parameter, and the tool normalizes your preset choice across all of them automatically.

When `"video_encoder"` is set to `"auto"` (the default), the tool picks the best available encoder and applies the correct quality parameters for that encoder.

## Configuration Location

Edit `configVideoConversion.json` (next to the executable)

## Quality Presets

### 1. **high_quality** (Original Default)
Prioritizes quality over file size. May result in larger files.

```json
"quality_preset": "high_quality"
```

**Use when:** You want maximum quality preservation, even if files get larger.

---

### 2. **balanced** (Recommended - New Default)
Balance between quality and file size reduction.

```json
"quality_preset": "balanced"
```

**Use when:** You want good quality with noticeable space savings (typical 20-40% reduction).

---

### 3. **space_saver**
Prioritizes file size reduction over quality.

```json
"quality_preset": "space_saver"
```

**Use when:** Storage space is critical and slight quality loss is acceptable (typical 40-60% reduction).

---

## Encoder Quality Parameters

Each encoder uses a different FFmpeg flag to control quality. The tool translates your preset into the correct values automatically.

### Encoder Comparison Table

| Encoder | Quality Flag | Flag Type | Preset: high_quality | Preset: balanced | Preset: space_saver |
|---------|-------------|-----------|---------------------|-----------------|-------------------|
| **libx265** (CPU) | `-crf` | Constant Rate Factor | 19-23 | 23-30 | 26-33 |
| **hevc_nvenc** (NVIDIA) | `-cq` | Constant Quality | 20-26 | 24-30 | 28-35 |
| **hevc_amf** (AMD) | `-qp_i`/`-qp_p` | Quantization Parameter | 16-22 | 20-27 | 24-31 |
| **hevc_qsv** (Intel) | `-global_quality` | ICQ (Intelligent CQ) | 17-23 | 21-28 | 25-33 |

Lower values = higher quality, larger files. Higher values = more compression, smaller files.

### libx265 (CPU) Quality Values

Uses `-crf` (Constant Rate Factor) with x265 encoding presets.

| Resolution | high_quality | balanced | space_saver |
|-----------|-------------|----------|-------------|
| SD (≤1024px) | CRF 19 | CRF 23 | CRF 26 |
| 720p (≤1280px) | CRF 20 | CRF 25 | CRF 28 |
| 1080p (≤1920px) | CRF 21 | CRF 27 | CRF 30 |
| 4K (>1920px) | CRF 23 | CRF 30 | CRF 33 |

**Encoding presets:** `slow` (high_quality), `medium` (balanced), `faster` (space_saver)

CRF is the standard quality metric for x265. Most people are familiar with these values. This encoder is the slowest but most compatible option.

---

### hevc_nvenc (NVIDIA GPU) Quality Values

Uses `-cq` (Constant Quality) with NVIDIA preset levels.

| Resolution | high_quality | balanced | space_saver |
|-----------|-------------|----------|-------------|
| SD (≤1024px) | CQ 20 | CQ 24 | CQ 28 |
| 720p (≤1280px) | CQ 22 | CQ 26 | CQ 30 |
| 1080p (≤1920px) | CQ 24 | CQ 28 | CQ 32 |
| 4K (>1920px) | CQ 26 | CQ 30 | CQ 35 |

**Encoding presets:** `p7` (high_quality), `p5` (balanced), `p4` (space_saver)

NVENC CQ values are roughly comparable to libx265 CRF, but not identical. NVENC is significantly faster than CPU encoding, with a small quality trade-off at the same numerical value.

---

### hevc_amf (AMD GPU) Quality Values

Uses `-rc cqp` rate control with `-qp_i` and `-qp_p` quantization parameters. Both I-frame and P-frame QP are set to the same value.

| Resolution | high_quality | balanced | space_saver |
|-----------|-------------|----------|-------------|
| SD (≤1024px) | QP 16 | QP 20 | QP 24 |
| 720p (≤1280px) | QP 18 | QP 22 | QP 26 |
| 1080p (≤1920px) | QP 20 | QP 24 | QP 28 |
| 4K (>1920px) | QP 22 | QP 27 | QP 31 |

**Quality presets:** `quality` (high_quality), `balanced` (balanced), `speed` (space_saver)

AMF QP values are on a different scale than CRF or CQ. Lower QP values produce higher quality. The numeric values are intentionally lower than libx265 CRF to produce comparable visual quality.

---

### hevc_qsv (Intel GPU) Quality Values

Uses `-global_quality` in ICQ (Intelligent Constant Quality) mode.

| Resolution | high_quality | balanced | space_saver |
|-----------|-------------|----------|-------------|
| SD (≤1024px) | ICQ 17 | ICQ 21 | ICQ 25 |
| 720p (≤1280px) | ICQ 19 | ICQ 23 | ICQ 27 |
| 1080p (≤1920px) | ICQ 21 | ICQ 25 | ICQ 30 |
| 4K (>1920px) | ICQ 23 | ICQ 28 | ICQ 33 |

**Encoding presets:** `veryslow` (high_quality), `medium` (balanced), `faster` (space_saver)

QSV's ICQ mode is Intel's equivalent of constant quality encoding. The values are roughly in the same ballpark as CRF, but the internal algorithm differs.

---

## Custom Quality Values

For fine-tuned control, set custom quality values per resolution. Custom values override presets.

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

Custom values are passed to whichever encoder is active. Keep in mind that the same number means different things on different encoders. A value of 24 on libx265 (CRF) isn't identical to 24 on NVENC (CQ) or AMF (QP).

---

## Understanding Quality Values

For **all encoders**, the general rule is:
- **Lower values (16-23)** = Higher quality, larger files
- **Medium values (24-28)** = Good quality, moderate file size
- **Higher values (29-35)** = Lower quality, smaller files

### Typical Results by Quality Range

| Range | Quality | Typical Size Change | Notes |
|-------|---------|---------------------|-------|
| 16-21 | Excellent | -10% to +20% | May increase size if source is already compressed |
| 22-25 | Very Good | -20% to -40% | Good balance for most content |
| 26-29 | Good | -40% to -60% | Noticeable compression, still acceptable |
| 30-33 | Fair | -60% to -70% | Visible artifacts on detailed scenes |
| 34+ | Poor | -70%+ | Not recommended |

These ranges are approximate and vary by encoder. GPU encoders may need slightly lower values than CPU to achieve equivalent visual quality.

---

## Example Configurations

### Recommended Default (Auto-Detect Encoder)
Let the tool pick the best encoder for your hardware:

```json
{
  "video_encoder": "auto",
  "quality_preset": "balanced"
}
```

### For Already-Compressed Videos
If files are getting larger, use **space_saver** or increase quality values:

```json
{
  "video_encoder": "auto",
  "quality_preset": "space_saver"
}
```

Or custom values:
```json
{
  "video_encoder": "auto",
  "quality_preset": "balanced",
  "custom_quality_1080p": 30
}
```

### Force CPU Encoding (Maximum Compatibility)
```json
{
  "video_encoder": "libx265",
  "quality_preset": "balanced"
}
```

### Force NVIDIA GPU Encoding
```json
{
  "video_encoder": "hevc_nvenc",
  "quality_preset": "balanced"
}
```

### For Archival Quality
Use **high_quality** to preserve maximum detail:

```json
{
  "video_encoder": "auto",
  "quality_preset": "high_quality"
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
2. Or increase custom quality values (28-32 for 1080p)

### Files are too small but quality is poor

**Problem:** Quality settings too aggressive.

**Solution:**
1. Change preset to `"balanced"` or `"high_quality"`
2. Or decrease custom quality values (23-26 for 1080p)

### Conversions always get discarded

**Problem:** All conversions result in larger files.

**Solution:**
- Source videos may already be HEVC at high compression
- Use `--force-hevc` to re-encode anyway
- Or skip these files (they're already optimal)

### GPU quality looks different than CPU

**Problem:** Same preset gives slightly different visual results on GPU vs CPU.

**Solution:**
This is expected. GPU encoders trade a small amount of quality for speed. If you need maximum quality at a given file size, use `--encoder libx265`. For most content, the difference is negligible at `balanced` or `high_quality` presets.

---

## Quick Reference

**Default config:**
```json
{
  "video_encoder": "auto",
  "quality_preset": "balanced",
  "custom_quality_sd": 0,
  "custom_quality_720p": 0,
  "custom_quality_1080p": 0,
  "custom_quality_4k": 0
}
```

**To get space savings on compressed files:**
```json
{
  "video_encoder": "auto",
  "quality_preset": "space_saver"
}
```

**For fine control:**
```json
{
  "video_encoder": "auto",
  "quality_preset": "balanced",
  "custom_quality_1080p": 30
}
```
