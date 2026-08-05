package runner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/config"
	"github.com/konono/trimetry/internal/id"
	"github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/telemetry"
	"github.com/konono/trimetry/internal/ui"
	"github.com/konono/trimetry/internal/version"
)

type Runner struct {
	Config    *config.Config
	Adapter   adapter.ApplicationAdapter
	Telemetry telemetry.Adapter
	Output    io.Writer
	Verbose   bool
	display   *ui.Display
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

	specs := r.expandTrials(run.BenchmarkRunID)

	w := r.Output
	if w == nil {
		w = os.Stdout
	}
	display := ui.NewDisplay(run, r.Config.Scenarios, r.Config.Models, r.Config.Benchmark.Trials, r.Config.Benchmark.Concurrency, w, r.Verbose)
	r.display = display

	executor := &Executor{
		Adapter:          r.Adapter,
		Telemetry:        r.Telemetry,
		Concurrency:      r.Config.Benchmark.Concurrency,
		Retries:          r.Config.Benchmark.Retries,
		FlushOnTrialEnd:  r.Config.Telemetry.FlushOnTrialEnd,
		EnrichmentDir:    r.Config.Telemetry.EnrichmentDir,
		BenchmarkName:    r.Config.Benchmark.Name,
		OnTrialComplete:  display.OnTrialComplete,
		OnRetry:          display.OnRetry,
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

	return run, nil
}

func (r *Runner) Display() *ui.Display {
	return r.display
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

