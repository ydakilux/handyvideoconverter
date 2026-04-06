package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/sirupsen/logrus"

	"video-converter/internal/database"
)

func defaultDBPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return "conversions.db"
	}
	return filepath.Join(filepath.Dir(exePath), "conversions.db")
}

func subcommandLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func openStore(dbPath string) (database.Store, func()) {
	store, err := database.NewSQLiteStore(dbPath, subcommandLogger())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	return store, func() { store.Close() }
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func RunStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	drive := fs.String("drive", "", "Filter by drive root (e.g. D:\\)")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	st, err := store.GetStats(context.Background(), *drive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	savedBytes := st.TotalOriginal - st.TotalConverted
	savedPct := 0.0
	if st.TotalOriginal > 0 {
		savedPct = float64(savedBytes) / float64(st.TotalOriginal) * 100
	}

	fmt.Println("Conversion Statistics")
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Total files:      %d\n", st.TotalFiles)
	fmt.Printf("  Successful:     %d\n", st.SuccessCount)
	fmt.Printf("  Errors:         %d\n", st.ErrorCount)
	fmt.Printf("  Not beneficial: %d\n", st.NotBeneficial)
	fmt.Printf("  Already HEVC:   %d\n", st.AlreadyHEVC)
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Total original:   %s\n", formatBytes(st.TotalOriginal))
	fmt.Printf("Total converted:  %s\n", formatBytes(st.TotalConverted))
	fmt.Printf("Space saved:      %s (%.1f%%)\n", formatBytes(savedBytes), savedPct)
}

func RunErrors(args []string) {
	fs := flag.NewFlagSet("errors", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	drive := fs.String("drive", "", "Filter by drive root (e.g. D:\\)")
	pathFilter := fs.String("path", "", "Filter by source path (SQL LIKE pattern, e.g. %Movies%)")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	records, err := store.GetErrors(context.Background(), *drive, *pathFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("No errors found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Hash\tDrive\tSource Path\tSize\tError\tDate\n")
	fmt.Fprintf(w, "────\t─────\t───────────\t────\t─────\t────\n")
	for _, r := range records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.FileHash, r.DriveRoot, r.SourcePath,
			formatBytes(r.OriginalSize), r.Error, r.ConvertedAt)
	}
	w.Flush()
}

func RunRecent(args []string) {
	fs := flag.NewFlagSet("recent", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	limit := fs.Int("limit", 10, "Number of recent records to show")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	records, err := store.GetRecent(context.Background(), *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("No data found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Drive\tSource Path\tOriginal\tConverted\tNote/Error\tCodec\tDate\n")
	fmt.Fprintf(w, "─────\t───────────\t────────\t─────────\t──────────\t─────\t────\n")
	for _, r := range records {
		noteOrErr := r.Note
		if r.Error != "" {
			noteOrErr = "ERR: " + r.Error
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.DriveRoot, r.SourcePath,
			formatBytes(r.OriginalSize), formatBytes(r.ConvertedSize),
			noteOrErr, r.SourceCodec, r.ConvertedAt)
	}
	w.Flush()
}

func RunNotBeneficial(args []string) {
	fs := flag.NewFlagSet("not-beneficial", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	drive := fs.String("drive", "", "Filter by drive root (e.g. D:\\)")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	records, err := store.GetNotBeneficial(context.Background(), *drive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("No not-beneficial conversions found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Drive\tSource Path\tOriginal\tConverted\tIncrease %%\n")
	fmt.Fprintf(w, "─────\t───────────\t────────\t─────────\t──────────\n")
	for _, r := range records {
		increase := 0.0
		if r.OriginalSize > 0 {
			increase = float64(r.ConvertedSize-r.OriginalSize) / float64(r.OriginalSize) * 100
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.1f%%\n",
			r.DriveRoot, r.SourcePath,
			formatBytes(r.OriginalSize), formatBytes(r.ConvertedSize),
			increase)
	}
	w.Flush()
}

func RunFormats(args []string) {
	fs := flag.NewFlagSet("formats", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	drive := fs.String("drive", "", "Filter by drive root (e.g. D:\\)")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	records, err := store.GetFormatBreakdown(context.Background(), *drive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("No data found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Codec\tContainer\tCount\tTotal Original\tTotal Converted\n")
	fmt.Fprintf(w, "─────\t─────────\t─────\t──────────────\t───────────────\n")
	for _, r := range records {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			r.SourceCodec, r.SourceContainer, r.Count,
			formatBytes(r.TotalOriginal), formatBytes(r.TotalConverted))
	}
	w.Flush()
}

func RunSpaceSaved(args []string) {
	fs := flag.NewFlagSet("space-saved", flag.ExitOnError)
	dbPath := fs.String("db-path", defaultDBPath(), "Path to SQLite database")
	period := fs.String("period", "total", "Time period: week, month, total")
	fs.Parse(args)

	store, cleanup := openStore(*dbPath)
	defer cleanup()

	result, err := store.GetSpaceSaved(context.Background(), *period)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Space Saved (%s)\n", result.Period)
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Files:      %d\n", result.FileCount)
	fmt.Printf("Saved:      %s\n", formatBytes(result.BytesSaved))
}
