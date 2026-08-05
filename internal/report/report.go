package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/konono/trimetry/internal/aggregator"
	"github.com/konono/trimetry/internal/fileutil"
	"github.com/konono/trimetry/internal/metrics"
	"github.com/konono/trimetry/internal/model"
)

type Generator struct {
	OutputDir  string
	Formats    []string
	MaskOutput bool
	Scenarios  []model.Scenario
}

func (g *Generator) Write(run *model.BenchmarkRun, manifest model.RunManifest) ([]model.ScenarioSummary, error) {
	dir := filepath.Join(g.OutputDir, run.BenchmarkRunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	if err := fileutil.WriteJSON(filepath.Join(dir, "run-manifest.json"), manifest); err != nil {
		return nil, err
	}

	if err := g.writeTrials(filepath.Join(dir, "trials.jsonl"), run.Trials); err != nil {
		return nil, err
	}

	if err := g.writeErrors(filepath.Join(dir, "errors.jsonl"), run.Trials); err != nil {
		return nil, err
	}

	summaries := g.buildSummaries(run.Trials, g.Scenarios)
	summaryData := model.RunSummary{
		BenchmarkRunID: run.BenchmarkRunID,
		Name:           run.Name,
		Status:         string(run.Status),
		StartedAt:      run.StartedAt,
		FinishedAt:     run.FinishedAt,
		Scenarios:      summaries,
	}

	for _, format := range g.Formats {
		switch format {
		case "json":
			if err := fileutil.WriteJSON(filepath.Join(dir, "summary.json"), summaryData); err != nil {
				return nil, err
			}
		case "markdown":
			if err := g.writeMarkdownSummary(filepath.Join(dir, "summary.md"), run, summaries); err != nil {
				return nil, err
			}
		}
	}

	return summaries, nil
}

func (g *Generator) writeTrials(path string, trials []model.Trial) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, t := range trials {
		trial := t
		if g.MaskOutput {
			trial.Output = "[MASKED]"
		}
		if err := enc.Encode(trial); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) writeErrors(path string, trials []model.Trial) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, t := range trials {
		if t.ExecutionStatus == model.ExecStatusCompleted {
			continue
		}
		entry := map[string]any{
			"trialId":         t.TrialID,
			"scenarioId":      t.ScenarioID,
			"trialNumber":     t.TrialNumber,
			"executionStatus": t.ExecutionStatus,
			"errorType":       t.ErrorType,
			"errorMessage":    t.ErrorMessage,
		}
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) buildSummaries(trials []model.Trial, scenarios []model.Scenario) []model.ScenarioSummary {
	scenarioMap := make(map[string]model.Scenario)
	for _, s := range scenarios {
		scenarioMap[s.ScenarioID] = s
	}

	groups := make(map[string][]model.Trial)
	for _, t := range trials {
		key := model.ScenarioModelKey(t.ScenarioID, t.ModelName)
		groups[key] = append(groups[key], t)
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var summaries []model.ScenarioSummary
	for _, key := range keys {
		group := groups[key]
		s := aggregator.Summarize(group)
		if sc, ok := scenarioMap[s.ScenarioID]; ok {
			s.ScenarioName = sc.Name
			s.Input = sc.Input
		}
		summaries = append(summaries, s)
	}
	return summaries
}

func (g *Generator) writeMarkdownSummary(path string, run *model.BenchmarkRun, summaries []model.ScenarioSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintf(w, "# Benchmark Report: %s\n\n", run.Name)
	fmt.Fprintf(w, "- **Run ID**: `%s`\n", run.BenchmarkRunID)
	fmt.Fprintf(w, "- **Status**: %s\n", run.Status)
	fmt.Fprintf(w, "- **Started**: %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
	if run.FinishedAt != nil {
		fmt.Fprintf(w, "- **Finished**: %s\n", run.FinishedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, "- **Duration**: %s\n", formatDuration(run.FinishedAt.Sub(run.StartedAt)))
	}
	fmt.Fprintf(w, "- **Environment**: %s\n", run.Environment)
	fmt.Fprintf(w, "- **Git Commit**: %s\n", run.GitCommit)
	fmt.Fprintf(w, "- **Config Hash**: %s\n", run.ConfigHash)
	fmt.Fprintf(w, "- **Runner Version**: %s\n\n", run.RunnerVersion)

	for _, s := range summaries {
		name := s.ScenarioID
		if s.ScenarioName != "" {
			name = s.ScenarioName
		}
		fmt.Fprintf(w, "## %s (model: %s)\n\n", name, s.ModelName)
		if s.Input != "" {
			fmt.Fprintf(w, "> **Prompt**: `%s`\n\n", s.Input)
		}
		fmt.Fprintf(w, "| Metric | Value |\n")
		fmt.Fprintf(w, "|--------|-------|\n")
		fmt.Fprintf(w, "| Trials | %d |\n", s.TrialCount)
		fmt.Fprintf(w, "| Completed | %d |\n", s.CompletedCount)
		fmt.Fprintf(w, "| Failed | %d |\n", s.FailedCount)
		fmt.Fprintf(w, "| Timeout | %d |\n", s.TimeoutCount)
		if s.CancelledCount > 0 {
			fmt.Fprintf(w, "| Cancelled | %d |\n", s.CancelledCount)
		}
		fmt.Fprintf(w, "| Failure Rate | %.1f%% |\n", s.FailureRate*100)
		if s.AccuracyRate != nil {
			fmt.Fprintf(w, "| Accuracy Rate | %.1f%% |\n", *s.AccuracyRate*100)
			fmt.Fprintf(w, "| Accuracy Method | %s |\n", s.AccuracyMethod)
		}
		fmt.Fprintln(w)

		if s.Metrics != nil {
			writeMetricTable(w, s.Metrics)
		}
		fmt.Fprintln(w)

		if len(s.ToolCalls) > 0 {
			fmt.Fprintf(w, "### Tool Calls\n\n")
			fmt.Fprintf(w, "| # | Tool | Command | Duration (ms) | Output |\n")
			fmt.Fprintf(w, "|---|------|---------|---------------|--------|\n")
			for i, tc := range s.ToolCalls {
				cmd := tc.Command
				if len(cmd) > 80 {
					cmd = cmd[:80] + "..."
				}
				out := tc.Output
				if len(out) > 50 {
					out = out[:50] + "..."
				}
				out = strings.ReplaceAll(out, "\n", " ")
				fmt.Fprintf(w, "| %d | %s | `%s` | %d | %s |\n", i+1, tc.Tool, cmd, tc.DurationMs, out)
			}
			fmt.Fprintln(w)
		}

		if len(s.OutputSamples) > 0 {
			fmt.Fprintf(w, "### Output Samples\n\n")
			for _, sample := range s.OutputSamples {
				mark := ""
				if sample.Accurate != nil {
					if *sample.Accurate {
						mark = " [ACCURATE]"
					} else {
						mark = " [INACCURATE]"
					}
				}
				fmt.Fprintf(w, "**Trial %d**%s:\n```\n%s\n```\n\n", sample.TrialNumber, mark, sample.Output)
			}
		}
	}

	return nil
}

func writeMetricTable(w *bufio.Writer, m *model.MetricsSummary) {
	fmt.Fprintf(w, "| Metric | Mean | Median | StdDev | Min | Max | P90 | P95 | N |\n")
	fmt.Fprintf(w, "|--------|------|--------|--------|-----|-----|-----|-----|---|\n")

	row := func(name string, s *model.StatsSummary) {
		if s == nil {
			return
		}
		f := formatMetricValue
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s | %s | %d |\n",
			name, f(s.Mean), f(s.Median), f(s.StdDev), f(s.Min), f(s.Max), f(s.P90), f(s.P95), s.Count)
	}

	for _, f := range metrics.Fields {
		row(f.Label, f.Extract(m))
	}
}

func formatMetricValue(v float64) string {
	if v == 0 {
		return "0.00"
	}
	abs := math.Abs(v)
	if abs >= 1 {
		return fmt.Sprintf("%.2f", v)
	}
	digits := int(math.Ceil(-math.Log10(abs))) + 2
	return fmt.Sprintf("%.*f", digits, v)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

