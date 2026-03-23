package encoder

import "strings"

// qualityTable holds quality parameter values for three presets × four resolution bands.
// Indices: [preset][band] where preset: 0=high_quality, 1=balanced/default, 2=space_saver
// and band: 0=SD(≤1024), 1=720p(≤1280), 2=1080p(≤1920), 3=4K(>1920).
type qualityTable [3][4]int

// qualityValue resolves a preset name and pixel width to a quality integer using
// the supplied table. Unknown presets fall back to the "balanced" row (index 1).
func qualityValue(preset string, width int, table qualityTable) int {
	var row int
	switch strings.ToLower(preset) {
	case "high_quality":
		row = 0
	case "space_saver":
		row = 2
	default: // "balanced" and any unrecognised preset
		row = 1
	}

	switch {
	case width <= 1024:
		return table[row][0]
	case width <= 1280:
		return table[row][1]
	case width <= 1920:
		return table[row][2]
	default:
		return table[row][3]
	}
}
