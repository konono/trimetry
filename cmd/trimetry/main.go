package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/comparator"
	"github.com/konono/trimetry/internal/config"
	"github.com/konono/trimetry/internal/report"
	"github.com/konono/trimetry/internal/runner"
	"github.com/konono/trimetry/internal/telemetry"
	"github.com/konono/trimetry/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(runBenchmark(os.Args[2:]))
	case "validate":
		os.Exit(runValidate(os.Args[2:]))
	case "compare":
		os.Exit(runCompare(os.Args[2:]))
	case "version":
		fmt.Println("trimetry", version.Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `trimetry %s - LLM agent benchmarking framework

Usage:
  trimetry <command> [options]

Commands:
  run           Run benchmarks
  validate      Validate a config file
  compare       Compare two benchmark runs
  version       Print version

Run Options:
  --config <path>   Config file path (required)
  --dry-run         Run in dry-run mode
  --verbose         Show per-trial detail lines in TTY mode

Validate Options:
  --config <path>   Config file path (required)

Compare Options:
  --baseline <path>     Baseline summary.json path (required)
  --candidate <path>    Candidate summary.json path (required)
`, version.Version)
}

func parseFlag(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func runBenchmark(args []string) int {
	configPath := parseFlag(args, "--config")
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		return 1
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		return 1
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "config validation failed:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return 1
	}

	dryRun := hasFlag(args, "--dry-run")
	if dryRun {
		cfg.ApplyDryRun()
	}

	app, err := adapter.NewAdapter(cfg.Adapter.Type, cfg.Adapter.Options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating adapter: %v\n", err)
		return 1
	}

	tel := telemetry.NewFromConfig(cfg)

	r := runner.New(cfg, app, tel)
	r.Verbose = hasFlag(args, "--verbose")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	run, err := r.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running benchmark: %v\n", err)
		return 1
	}

	manifest := r.BuildManifest(run, dryRun)

	gen := &report.Generator{
		OutputDir:  cfg.Report.OutputDirectory,
		Formats:    cfg.Report.Formats,
		MaskOutput: cfg.Report.MaskOutput,
		Scenarios:  cfg.Scenarios,
	}
	summaries, err := gen.Write(run, manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing report: %v\n", err)
		return 1
	}

	r.Display().Finalize(run, summaries, cfg.Report.OutputDirectory)
	return 0
}

func runValidate(args []string) int {
	configPath := parseFlag(args, "--config")
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		return 1
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		return 1
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "config validation failed:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return 1
	}

	fmt.Println("config is valid")
	return 0
}

func runCompare(args []string) int {
	baselinePath := parseFlag(args, "--baseline")
	candidatePath := parseFlag(args, "--candidate")
	if baselinePath == "" || candidatePath == "" {
		fmt.Fprintln(os.Stderr, "error: --baseline and --candidate are required")
		return 1
	}

	baseline, err := comparator.LoadSummary(baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading baseline: %v\n", err)
		return 1
	}

	candidate, err := comparator.LoadSummary(candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading candidate: %v\n", err)
		return 1
	}

	report := comparator.Compare(baseline, candidate)
	comparator.Print(report)
	return 0
}

