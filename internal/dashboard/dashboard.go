package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ydakilux/reforge/internal/database"
)

type DashboardData struct {
	GeneratedAt     string                         `json:"generatedAt"`
	Stats           *database.Stats                `json:"stats"`
	DriveRoots      []string                       `json:"driveRoots"`
	Errors          []database.ErrorRecord         `json:"errors"`
	Recent          []database.RecentRecord        `json:"recent"`
	NotBeneficial   []database.NotBeneficialRecord `json:"notBeneficial"`
	Formats         []database.FormatStat          `json:"formats"`
	SpaceSaved      *database.SpaceSavedResult     `json:"spaceSaved"`
	SpaceSavedWeek  *database.SpaceSavedResult     `json:"spaceSavedWeek"`
	SpaceSavedMonth *database.SpaceSavedResult     `json:"spaceSavedMonth"`
	Timeline        []database.TimelinePoint       `json:"timeline"`
}

func collectData(ctx context.Context, store database.Store) (*DashboardData, error) {
	data := &DashboardData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	var err error

	data.DriveRoots, err = store.GetDriveRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("get drive roots: %w", err)
	}

	data.Stats, err = store.GetStats(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	data.Errors, err = store.GetErrors(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("get errors: %w", err)
	}

	data.Recent, err = store.GetRecent(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("get recent: %w", err)
	}

	data.NotBeneficial, err = store.GetNotBeneficial(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get not beneficial: %w", err)
	}

	data.Formats, err = store.GetFormatBreakdown(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get formats: %w", err)
	}

	data.SpaceSaved, err = store.GetSpaceSaved(ctx, "total")
	if err != nil {
		return nil, fmt.Errorf("get space saved total: %w", err)
	}

	data.SpaceSavedWeek, err = store.GetSpaceSaved(ctx, "week")
	if err != nil {
		return nil, fmt.Errorf("get space saved week: %w", err)
	}

	data.SpaceSavedMonth, err = store.GetSpaceSaved(ctx, "month")
	if err != nil {
		return nil, fmt.Errorf("get space saved month: %w", err)
	}

	data.Timeline, err = store.GetConversionTimeline(ctx)
	if err != nil {
		return nil, fmt.Errorf("get timeline: %w", err)
	}

	return data, nil
}

func Generate(ctx context.Context, store database.Store, outputPath string) (string, error) {
	data, err := collectData(ctx, store)
	if err != nil {
		return "", fmt.Errorf("collect dashboard data: %w", err)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal dashboard data: %w", err)
	}

	html := renderHTML(string(jsonBytes))

	if outputPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			outputPath = "dashboard.html"
		} else {
			outputPath = filepath.Join(filepath.Dir(exePath), "dashboard.html")
		}
	}

	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("write dashboard: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename dashboard: %w", err)
	}

	return outputPath, nil
}
