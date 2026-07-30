package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/config"
	"github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/report"
	"github.com/konono/trimetry/internal/telemetry"
)

func TestE2EFakeRun(t *testing.T) {
	cfg := &config.Config{
		Benchmark: config.BenchmarkConfig{
			Name:           "e2e-test",
			Trials:         3,
			Concurrency:    2,
			TimeoutSeconds: 30,
			Environment:    "test",
		},
		Scenarios: []model.Scenario{
			{
				ScenarioID:      "greeting",
				ScenarioVersion: "1",
				Name:            "Greeting",
				Input:           "Hello",
				TimeoutSeconds:  30,
			},
			{
				ScenarioID:      "tool-test",
				ScenarioVersion: "1",
				Name:            "Tool Test",
				Input:           "List files",
				TimeoutSeconds:  30,
			},
		},
		Models: []model.ModelConfig{
			{Name: "fake-model", Provider: "fake"},
		},
		Telemetry: config.TelemetryConfig{Enabled: false},
		Report: config.ReportConfig{
			OutputDirectory: t.TempDir(),
			Formats:         []string{"json", "markdown"},
		},
	}

	app := adapter.NewFakeAdapter()
	tel := &telemetry.NoopAdapter{}

	r := New(cfg, app, tel)
	run, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if run.Status != model.RunStatusCompleted {
		t.Errorf("expected completed, got %s", run.Status)
	}

	// 2 scenarios × 1 model × 3 trials = 6
	if len(run.Trials) != 6 {
		t.Errorf("expected 6 trials, got %d", len(run.Trials))
	}

	for _, trial := range run.Trials {
		if trial.ExecutionStatus != model.ExecStatusCompleted {
			t.Errorf("trial %s: expected completed, got %s", trial.TrialID, trial.ExecutionStatus)
		}
		if trial.Metrics == nil {
			t.Errorf("trial %s: expected metrics", trial.TrialID)
			continue
		}
		if trial.Metrics.WallTimeMs <= 0 {
			t.Errorf("trial %s: expected positive wall time", trial.TrialID)
		}
		if trial.Metrics.InputTokens == nil {
			t.Errorf("trial %s: expected input tokens from fake adapter", trial.TrialID)
		}
	}

	// verify report generation
	gen := &report.Generator{
		OutputDir: cfg.Report.OutputDirectory,
		Formats:   cfg.Report.Formats,
	}
	manifest := r.BuildManifest(run, false)
	if err := gen.Write(run, manifest); err != nil {
		t.Fatalf("Report write failed: %v", err)
	}

	dir := filepath.Join(cfg.Report.OutputDirectory, run.BenchmarkRunID)
	for _, name := range []string{"run-manifest.json", "trials.jsonl", "summary.json", "summary.md", "errors.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s to exist: %v", name, err)
		}
	}
}

func TestE2ETrialIsolation(t *testing.T) {
	fa := adapter.NewFakeAdapter()
	fa.FailRate = 0.5

	cfg := &config.Config{
		Benchmark: config.BenchmarkConfig{
			Name:           "isolation-test",
			Trials:         10,
			Concurrency:    3,
			TimeoutSeconds: 10,
			Environment:    "test",
		},
		Scenarios: []model.Scenario{
			{
				ScenarioID:      "flaky",
				ScenarioVersion: "1",
				Input:           "test",
				TimeoutSeconds:  10,
			},
		},
		Models: []model.ModelConfig{
			{Name: "fake-model", Provider: "fake"},
		},
		Telemetry: config.TelemetryConfig{Enabled: false},
		Report: config.ReportConfig{
			OutputDirectory: t.TempDir(),
			Formats:         []string{"json"},
		},
	}

	r := New(cfg, fa, &telemetry.NoopAdapter{})
	run, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(run.Trials) != 10 {
		t.Fatalf("expected 10 trials, got %d", len(run.Trials))
	}

	// with 50% fail rate over 10 trials, we should see both completed and failed
	completed := 0
	failed := 0
	for _, trial := range run.Trials {
		switch trial.ExecutionStatus {
		case model.ExecStatusCompleted:
			completed++
		case model.ExecStatusFailed:
			failed++
		}
		// every trial should have metrics regardless of status
		if trial.Metrics == nil {
			t.Errorf("trial %s: expected metrics even on failure", trial.TrialID)
		}
	}

	if completed == 0 {
		t.Error("expected at least one completed trial")
	}
	if failed == 0 {
		t.Error("expected at least one failed trial (50% fail rate)")
	}

	// run should complete even with failures
	if run.Status != model.RunStatusCompleted {
		t.Errorf("expected run to complete despite trial failures, got %s", run.Status)
	}
}
