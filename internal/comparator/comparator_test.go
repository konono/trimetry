package comparator

import (
	"strings"
	"testing"

	"github.com/konono/trimetry/internal/model"
)

func makeStats(mean, median, stddev, min, max, p90, p95 float64, count int) *model.StatsSummary {
	return &model.StatsSummary{
		Mean: mean, Median: median, StdDev: stddev,
		Min: min, Max: max, P90: p90, P95: p95, Count: count,
	}
}

func makeScenario(id, version, name, modelName string, trialCount int, metrics *model.MetricsSummary) model.ScenarioSummary {
	return model.ScenarioSummary{
		ScenarioID:      id,
		ScenarioVersion: version,
		ScenarioName:    name,
		ModelName:       modelName,
		ModelProvider:   "test-provider",
		TrialCount:      trialCount,
		CompletedCount:  trialCount,
		FailedCount:     0,
		TimeoutCount:    0,
		FailureRate:     0.0,
		Metrics:         metrics,
	}
}

func TestCompareBasic(t *testing.T) {
	bMetrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 5),
	}
	cMetrics := &model.MetricsSummary{
		WallTimeMs: makeStats(90, 85, 8, 70, 110, 100, 105, 5),
	}

	baseline := &model.RunSummary{
		Name:      "baseline-run",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "1", "Sort Test", "model-a", 5, bMetrics)},
	}
	candidate := &model.RunSummary{
		Name:      "candidate-run",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "1", "Sort Test", "model-a", 5, cMetrics)},
	}

	report := Compare(baseline, candidate)

	if len(report.Comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(report.Comparisons))
	}
	comp := report.Comparisons[0]
	if comp.ScenarioID != "s1" {
		t.Errorf("scenarioID = %q, want %q", comp.ScenarioID, "s1")
	}
	if len(comp.Diffs) == 0 {
		t.Error("expected diffs to be populated")
	}
	// Verify the wallTimeMs diff
	found := false
	for _, d := range comp.Diffs {
		if d.Metric == "wallTimeMs" {
			found = true
			if d.BaselineMean != 100 {
				t.Errorf("baseline mean = %v, want 100", d.BaselineMean)
			}
			if d.CandidateMean != 90 {
				t.Errorf("candidate mean = %v, want 90", d.CandidateMean)
			}
		}
	}
	if !found {
		t.Error("wallTimeMs diff not found")
	}
}

func TestCompareVersionMismatch(t *testing.T) {
	metrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 5),
	}

	baseline := &model.RunSummary{
		Name:      "baseline",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "1", "Sort Test", "model-a", 5, metrics)},
	}
	candidate := &model.RunSummary{
		Name:      "candidate",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "2", "Sort Test", "model-a", 5, metrics)},
	}

	report := Compare(baseline, candidate)

	found := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "version differs") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected version mismatch warning, got warnings: %v", report.Warnings)
	}
}

func TestCompareMissingScenario(t *testing.T) {
	metrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 5),
	}

	baseline := &model.RunSummary{
		Name:      "baseline",
		Scenarios: []model.ScenarioSummary{makeScenario("s-base", "1", "Base Only", "model-a", 5, metrics)},
	}
	candidate := &model.RunSummary{
		Name:      "candidate",
		Scenarios: []model.ScenarioSummary{makeScenario("s-cand", "1", "Cand Only", "model-a", 5, metrics)},
	}

	report := Compare(baseline, candidate)

	if len(report.Comparisons) != 0 {
		t.Errorf("expected 0 comparisons, got %d", len(report.Comparisons))
	}

	baseWarning := false
	candWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "s-base") && strings.Contains(w, "not found in candidate") {
			baseWarning = true
		}
		if strings.Contains(w, "s-cand") && strings.Contains(w, "not found in baseline") {
			candWarning = true
		}
	}
	if !baseWarning {
		t.Errorf("expected warning about s-base missing in candidate, got: %v", report.Warnings)
	}
	if !candWarning {
		t.Errorf("expected warning about s-cand missing in baseline, got: %v", report.Warnings)
	}
}

func TestCompareSampleSizeWarning(t *testing.T) {
	metrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 3),
	}

	baseline := &model.RunSummary{
		Name:      "baseline",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "1", "Sort", "model-a", 3, metrics)},
	}
	candidate := &model.RunSummary{
		Name:      "candidate",
		Scenarios: []model.ScenarioSummary{makeScenario("s1", "1", "Sort", "model-a", 4, metrics)},
	}

	report := Compare(baseline, candidate)

	found := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "sample size < 5") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sample size warning, got: %v", report.Warnings)
	}
}

func TestCompareCancelledCount(t *testing.T) {
	metrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 5),
	}

	bScenario := makeScenario("s1", "1", "Sort Test", "model-a", 5, metrics)
	bScenario.CancelledCount = 2
	bScenario.CompletedCount = 3

	cScenario := makeScenario("s1", "1", "Sort Test", "model-a", 5, metrics)
	cScenario.CancelledCount = 0

	baseline := &model.RunSummary{
		Name:      "baseline-run",
		Scenarios: []model.ScenarioSummary{bScenario},
	}
	candidate := &model.RunSummary{
		Name:      "candidate-run",
		Scenarios: []model.ScenarioSummary{cScenario},
	}

	report := Compare(baseline, candidate)

	if len(report.Comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(report.Comparisons))
	}
	comp := report.Comparisons[0]
	if comp.Baseline.CancelledCount != 2 {
		t.Errorf("Baseline.CancelledCount = %d, want 2", comp.Baseline.CancelledCount)
	}
	if comp.Candidate.CancelledCount != 0 {
		t.Errorf("Candidate.CancelledCount = %d, want 0", comp.Candidate.CancelledCount)
	}
}

func TestCompareSorted(t *testing.T) {
	metrics := &model.MetricsSummary{
		WallTimeMs: makeStats(100, 95, 10, 80, 120, 110, 115, 5),
	}

	baseline := &model.RunSummary{
		Name: "baseline",
		Scenarios: []model.ScenarioSummary{
			makeScenario("z-scenario", "1", "Z", "model-a", 5, metrics),
			makeScenario("a-scenario", "1", "A", "model-a", 5, metrics),
			makeScenario("m-scenario", "1", "M", "model-a", 5, metrics),
		},
	}
	candidate := &model.RunSummary{
		Name: "candidate",
		Scenarios: []model.ScenarioSummary{
			makeScenario("z-scenario", "1", "Z", "model-a", 5, metrics),
			makeScenario("a-scenario", "1", "A", "model-a", 5, metrics),
			makeScenario("m-scenario", "1", "M", "model-a", 5, metrics),
		},
	}

	report := Compare(baseline, candidate)

	if len(report.Comparisons) != 3 {
		t.Fatalf("expected 3 comparisons, got %d", len(report.Comparisons))
	}
	expected := []string{"a-scenario", "m-scenario", "z-scenario"}
	for i, want := range expected {
		if report.Comparisons[i].ScenarioID != want {
			t.Errorf("comparisons[%d].ScenarioID = %q, want %q", i, report.Comparisons[i].ScenarioID, want)
		}
	}
}
