package aggregator

import (
	"math"
	"sort"

	"github.com/konono/trimetry/internal/metrics"
	"github.com/konono/trimetry/internal/model"
)

const maxOutputLen = 500

func Summarize(trials []model.Trial) model.ScenarioSummary {
	if len(trials) == 0 {
		return model.ScenarioSummary{}
	}

	first := trials[0]
	summary := model.ScenarioSummary{
		ScenarioID:      first.ScenarioID,
		ScenarioVersion: first.ScenarioVersion,
		ModelName:       first.ModelName,
		ModelProvider:    first.ModelProvider,
		TrialCount:      len(trials),
	}

	summary.CompletedCount, summary.FailedCount, summary.TimeoutCount, summary.CancelledCount = model.CountTrialStatuses(trials)

	accurateCount := 0
	hasAccuracy := false

	for _, t := range trials {
		if t.Metrics != nil && t.Metrics.AccuracyScore != nil {
			hasAccuracy = true
			if *t.Metrics.AccuracyScore >= 1.0 {
				accurateCount++
			}
		}
	}

	if summary.TrialCount > 0 {
		summary.FailureRate = float64(summary.FailedCount+summary.TimeoutCount+summary.CancelledCount) / float64(summary.TrialCount)
	}

	if hasAccuracy && summary.TrialCount > 0 {
		rate := float64(accurateCount) / float64(summary.TrialCount)
		summary.AccuracyRate = &rate
		for _, t := range trials {
			if t.Metrics != nil && t.Metrics.AccuracyMethod != "" {
				summary.AccuracyMethod = t.Metrics.AccuracyMethod
				break
			}
		}
	}

	summary.OutputSamples = collectOutputSamples(trials)
	summary.ToolCalls = collectToolCalls(trials)
	summary.Metrics = aggregateMetrics(trials)
	return summary
}

func collectOutputSamples(trials []model.Trial) []model.OutputSample {
	var samples []model.OutputSample
	for _, t := range trials {
		if t.Output == "" {
			continue
		}
		output := t.Output
		if len(output) > maxOutputLen {
			output = output[:maxOutputLen] + "..."
		}
		sample := model.OutputSample{
			TrialNumber: t.TrialNumber,
			Output:      output,
		}
		if t.Metrics != nil && t.Metrics.AccuracyScore != nil {
			accurate := *t.Metrics.AccuracyScore >= 1.0
			sample.Accurate = &accurate
		}
		samples = append(samples, sample)
	}
	return samples
}

func collectToolCalls(trials []model.Trial) []model.ToolCall {
	var all []model.ToolCall
	for _, t := range trials {
		if t.Metrics == nil {
			continue
		}
		all = append(all, t.Metrics.ToolCalls...)
	}
	const maxToolCalls = 50
	if len(all) > maxToolCalls {
		all = all[:maxToolCalls]
	}
	return all
}

func aggregateMetrics(trials []model.Trial) *model.MetricsSummary {
	slices := make(map[string][]float64, len(metrics.Fields))

	for _, t := range trials {
		if t.Metrics == nil {
			continue
		}
		for _, f := range metrics.Fields {
			if v := f.Collect(t.Metrics); v != nil {
				slices[f.Key] = append(slices[f.Key], *v)
			}
		}
	}

	ms := &model.MetricsSummary{}
	for _, f := range metrics.Fields {
		if vals := slices[f.Key]; len(vals) > 0 {
			s := computeStats(vals)
			f.Set(ms, &s)
		}
	}
	return ms
}

func computeStats(values []float64) model.StatsSummary {
	n := len(values)
	if n == 0 {
		return model.StatsSummary{}
	}

	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(n)

	variance := 0.0
	for _, v := range sorted {
		d := v - mean
		variance += d * d
	}
	if n > 1 {
		variance /= float64(n - 1)
	}

	return model.StatsSummary{
		Mean:   roundSig(mean),
		Median: percentile(sorted, 50),
		StdDev: roundSig(math.Sqrt(variance)),
		Min:    sorted[0],
		Max:    sorted[n-1],
		P90:    percentile(sorted, 90),
		P95:    percentile(sorted, 95),
		Count:  n,
	}
}

func roundSig(v float64) float64 {
	if v == 0 {
		return 0
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	digits := 2 - int(math.Floor(math.Log10(math.Abs(v))))
	if digits < 2 {
		digits = 2
	}
	shift := math.Pow(10, float64(digits))
	return math.Round(v*shift) / shift
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}

	rank := (p / 100) * float64(n-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}

	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
