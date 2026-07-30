package runner

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/config"
	"github.com/konono/trimetry/internal/version"
	"github.com/konono/trimetry/internal/id"
	"github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/telemetry"
)

type Runner struct {
	Config    *config.Config
	Adapter   adapter.ApplicationAdapter
	Telemetry telemetry.Adapter
}

func New(cfg *config.Config, app adapter.ApplicationAdapter, tel telemetry.Adapter) *Runner {
	return &Runner{
		Config:    cfg,
		Adapter:   app,
		Telemetry: tel,
	}
}

func (r *Runner) Run(ctx context.Context) (*model.BenchmarkRun, error) {
	run := &model.BenchmarkRun{
		BenchmarkRunID: id.NewBenchmarkRunID(),
		Name:           r.Config.Benchmark.Name,
		StartedAt:      time.Now(),
		RunnerVersion:  version.Version,
		Environment:    r.Config.Benchmark.Environment,
		GitCommit:      detectGitCommit(),
		ConfigHash:     r.Config.Hash(),
		Status:         model.RunStatusRunning,
	}

	log.Printf("Benchmark Run: %s (%s)", run.Name, run.BenchmarkRunID)
	log.Printf("  Environment: %s", run.Environment)
	log.Printf("  Git Commit:  %s", run.GitCommit)

	specs := r.expandTrials(run.BenchmarkRunID)
	log.Printf("  Scenarios:   %d", len(r.Config.Scenarios))
	log.Printf("  Models:      %d", len(r.Config.Models))
	log.Printf("  Trials/Combo: %d", r.Config.Benchmark.Trials)
	log.Printf("  Total Trials: %d", len(specs))
	log.Printf("  Concurrency: %d", r.Config.Benchmark.Concurrency)

	executor := &Executor{
		Adapter:         r.Adapter,
		Telemetry:       r.Telemetry,
		Concurrency:     r.Config.Benchmark.Concurrency,
		Retries:         r.Config.Benchmark.Retries,
		FlushOnTrialEnd: r.Config.Telemetry.FlushOnTrialEnd,
		EnrichmentDir:   r.Config.Telemetry.EnrichmentDir,
		BenchmarkName:   r.Config.Benchmark.Name,
	}

	trials := executor.RunTrials(ctx, specs)
	run.Trials = trials

	now := time.Now()
	run.FinishedAt = &now

	// Run status is always "completed" — it indicates the benchmark run
	// itself finished, not that all trials succeeded. Individual trial
	// failures are tracked in each trial's ExecutionStatus.
	run.Status = model.RunStatusCompleted

	r.Telemetry.Flush()

	log.Printf("Benchmark Run finished: %s (status=%s, duration=%v)",
		run.BenchmarkRunID, run.Status, run.FinishedAt.Sub(run.StartedAt))

	return run, nil
}

func (r *Runner) expandTrials(benchmarkRunID string) []TrialSpec {
	var specs []TrialSpec

	for _, scenario := range r.Config.Scenarios {
		for _, mdl := range r.Config.Models {
			for i := 1; i <= r.Config.Benchmark.Trials; i++ {
				trial := model.Trial{
					TrialID:         id.NewTrialID(),
					BenchmarkRunID:  benchmarkRunID,
					ScenarioID:      scenario.ScenarioID,
					ScenarioVersion: scenario.ScenarioVersion,
					TrialNumber:     i,
					ModelName:       mdl.Name,
					ModelProvider:    mdl.Provider,
					ModelParameters: mdl.Parameters,
					ExecutionStatus: model.ExecStatusPending,
				}

				specs = append(specs, TrialSpec{
					Trial:    trial,
					Scenario: scenario,
					Input:    scenario.Input,
					Pricing:  mdl.Pricing,
				})
			}
		}
	}

	return specs
}

func detectGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (r *Runner) BuildManifest(run *model.BenchmarkRun, dryRun bool) model.RunManifest {
	telCfg := map[string]any{
		"enabled":  r.Config.Telemetry.Enabled,
		"provider": r.Config.Telemetry.Provider,
	}
	if r.Config.Telemetry.BaseURL != "" {
		telCfg["baseUrl"] = r.Config.Telemetry.BaseURL
	}
	if r.Config.Telemetry.TrackingURI != "" {
		telCfg["trackingUri"] = r.Config.Telemetry.TrackingURI
	}
	if r.Config.Telemetry.Workspace != "" {
		telCfg["workspace"] = r.Config.Telemetry.Workspace
	}

	adapterCfg := map[string]any{
		"type": r.Config.Adapter.Type,
	}
	if len(r.Config.Adapter.Options) > 0 {
		safe := r.Config.RedactOptions()
		adapterCfg["options"] = safe
	}

	return model.RunManifest{
		BenchmarkRunID: run.BenchmarkRunID,
		Name:           run.Name,
		StartedAt:      run.StartedAt,
		RunnerVersion:  run.RunnerVersion,
		Environment:    run.Environment,
		GitCommit:      run.GitCommit,
		ConfigHash:          run.ConfigHash,
		EffectiveConfigHash: r.Config.EffectiveHash(),
		Scenarios:      r.Config.Scenarios,
		Models:         r.Config.Models,
		TrialsPerCombo:     r.Config.Benchmark.Trials,
		Concurrency:    r.Config.Benchmark.Concurrency,
		Retries:        r.Config.Benchmark.Retries,
		Telemetry:      telCfg,
		Adapter:        adapterCfg,
		DryRun:         dryRun,
	}
}

