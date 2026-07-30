package comparator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/konono/trimetry/internal/metrics"
	"github.com/konono/trimetry/internal/model"
)

type ComparisonReport struct {
	Baseline       string               `json:"baseline"`
	BaselineRunID  string               `json:"baselineRunId,omitempty"`
	Candidate      string               `json:"candidate"`
	CandidateRunID string               `json:"candidateRunId,omitempty"`
	Comparisons    []ScenarioComparison `json:"comparisons"`
	Warnings       []string             `json:"warnings,omitempty"`
}

type ScenarioComparison struct {
	ScenarioID     string            `json:"scenarioId"`
	BaselineModel  string            `json:"baselineModel"`
	CandidateModel string            `json:"candidateModel"`
	Baseline       ComparisonSide    `json:"baseline"`
	Candidate      ComparisonSide    `json:"candidate"`
	Diffs          []MetricDiff      `json:"diffs"`
}

type ComparisonSide struct {
	TrialCount     int      `json:"trialCount"`
	CompletedCount int      `json:"completedCount"`
	FailedCount    int      `json:"failedCount"`
	TimeoutCount   int      `json:"timeoutCount"`
	CancelledCount int      `json:"cancelledCount,omitempty"`
	FailureRate    float64  `json:"failureRate"`
	AccuracyRate   *float64 `json:"accuracyRate,omitempty"`
}

type MetricDiff struct {
	Metric        string  `json:"metric"`
	BaselineMean  float64 `json:"baselineMean"`
	CandidateMean float64 `json:"candidateMean"`
	AbsDiff       float64 `json:"absDiff"`
	ChangePercent float64 `json:"changePercent"`
	BaselineP50   float64 `json:"baselineP50"`
	CandidateP50  float64 `json:"candidateP50"`
	BaselineP90   float64 `json:"baselineP90"`
	CandidateP90  float64 `json:"candidateP90"`
	BaselineP95   float64 `json:"baselineP95"`
	CandidateP95  float64 `json:"candidateP95"`
	BaselineStdDev float64 `json:"baselineStdDev"`
	CandidateStdDev float64 `json:"candidateStdDev"`
	BaselineMin   float64 `json:"baselineMin"`
	CandidateMin  float64 `json:"candidateMin"`
	BaselineMax   float64 `json:"baselineMax"`
	CandidateMax  float64 `json:"candidateMax"`
}

func LoadSummary(path string) (*model.RunSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var input model.RunSummary
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &input, nil
}

func Compare(baseline, candidate *model.RunSummary) *ComparisonReport {
	report := &ComparisonReport{
		Baseline:       baseline.Name,
		BaselineRunID:  baseline.BenchmarkRunID,
		Candidate:      candidate.Name,
		CandidateRunID: candidate.BenchmarkRunID,
	}

	baseMap := indexScenarios(baseline.Scenarios)
	candMap := indexScenarios(candidate.Scenarios)

	for key, bs := range baseMap {
		cs, ok := candMap[key]
		if !ok {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("scenario %s not found in candidate", key))
			continue
		}

		if bs.ScenarioVersion != cs.ScenarioVersion {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("scenario %s: version differs between baseline (%s) and candidate (%s); results may not be directly comparable",
					bs.ScenarioID, bs.ScenarioVersion, cs.ScenarioVersion))
		}

		sc := ScenarioComparison{
			ScenarioID:     bs.ScenarioID,
			BaselineModel:  bs.ModelName,
			CandidateModel: cs.ModelName,
			Baseline: ComparisonSide{
				TrialCount:     bs.TrialCount,
				CompletedCount: bs.CompletedCount,
				FailedCount:    bs.FailedCount,
				TimeoutCount:   bs.TimeoutCount,
				CancelledCount: bs.CancelledCount,
				FailureRate:    bs.FailureRate,
				AccuracyRate:   bs.AccuracyRate,
			},
			Candidate: ComparisonSide{
				TrialCount:     cs.TrialCount,
				CompletedCount: cs.CompletedCount,
				FailedCount:    cs.FailedCount,
				TimeoutCount:   cs.TimeoutCount,
				CancelledCount: cs.CancelledCount,
				FailureRate:    cs.FailureRate,
				AccuracyRate:   cs.AccuracyRate,
			},
		}

		if bs.Metrics != nil && cs.Metrics != nil {
			sc.Diffs = compareMetrics(bs.Metrics, cs.Metrics)
		}

		report.Comparisons = append(report.Comparisons, sc)
	}

	for key := range candMap {
		if _, ok := baseMap[key]; !ok {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("scenario %s not found in baseline", key))
		}
	}

	sort.Slice(report.Comparisons, func(i, j int) bool {
		ki := model.ScenarioModelKey(report.Comparisons[i].ScenarioID, report.Comparisons[i].BaselineModel)
		kj := model.ScenarioModelKey(report.Comparisons[j].ScenarioID, report.Comparisons[j].BaselineModel)
		return ki < kj
	})

	for _, sc := range report.Comparisons {
		if sc.Baseline.TrialCount < 5 || sc.Candidate.TrialCount < 5 {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("scenario %s: sample size < 5 (baseline=%d, candidate=%d); statistical significance is limited",
					sc.ScenarioID, sc.Baseline.TrialCount, sc.Candidate.TrialCount))
		}
	}

	return report
}

func indexScenarios(summaries []model.ScenarioSummary) map[string]model.ScenarioSummary {
	m := make(map[string]model.ScenarioSummary)
	for _, s := range summaries {
		m[model.ScenarioModelKey(s.ScenarioID, s.ModelName)] = s
	}
	return m
}

func compareMetrics(bs, cs *model.MetricsSummary) []MetricDiff {
	var diffs []MetricDiff

	add := func(name string, b, c *model.StatsSummary) {
		if b == nil || c == nil {
			return
		}
		absDiff := c.Mean - b.Mean
		changePct := 0.0
		if b.Mean != 0 {
			changePct = (absDiff / b.Mean) * 100
		}
		diffs = append(diffs, MetricDiff{
			Metric:          name,
			BaselineMean:    b.Mean,
			CandidateMean:   c.Mean,
			AbsDiff:         absDiff,
			ChangePercent:   changePct,
			BaselineP50:     b.Median,
			CandidateP50:    c.Median,
			BaselineP90:     b.P90,
			CandidateP90:    c.P90,
			BaselineP95:     b.P95,
			CandidateP95:    c.P95,
			BaselineStdDev:  b.StdDev,
			CandidateStdDev: c.StdDev,
			BaselineMin:     b.Min,
			CandidateMin:    c.Min,
			BaselineMax:     b.Max,
			CandidateMax:    c.Max,
		})
	}

	for _, f := range metrics.Fields {
		add(f.Key, f.Extract(bs), f.Extract(cs))
	}

	return diffs
}
