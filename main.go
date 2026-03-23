package main

import (
	"flag"
	"fmt"
	"os"

	"video-converter/internal/app"
)

func main() {
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

	a, err := app.New(app.Options{
		ConfigFile:     *configFile,
		DryRun:         *dryRun,
		Bypass:         *bypassFlag,
		ForceHevc:      *forceHevcFlag,
		SameDrive:      *sameDrive,
		EncoderName:    *encoderFlag,
		ParallelJobs:   *parallelJobs,
		NonInteractive: *nonInteractive,
		Rebenchmark:    *rebenchmark,
		Paths:          flag.Args(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		app.ExitWithPause(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		app.ExitWithPause(1)
	}
}
