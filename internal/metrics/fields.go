package metrics

import "github.com/konono/trimetry/internal/model"

// Field defines a single metric for aggregation, comparison, and reporting.
// Adding a new metric requires only updating MetricsSummary + appending to Fields.
type Field struct {
	Key     string                                             // JSON key (e.g. "wallTimeMs")
	Label   string                                             // Display label (e.g. "Wall Time (ms)")
	Extract func(*model.MetricsSummary) *model.StatsSummary    // read from summary
	Set     func(*model.MetricsSummary, *model.StatsSummary)   // write to summary
	Collect func(*model.TrialMetrics) *float64                 // extract raw value from trial
}

// Helper functions to reduce Collect boilerplate.
func collectInt64(v *int64) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

func collectFloat64(v *float64) *float64 {
	return v
}

func collectInt(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// Fields is the single source of truth for all metric fields.
var Fields = []Field{
	{
		Key:     "wallTimeMs",
		Label:   "Wall Time (ms)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.WallTimeMs },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.WallTimeMs = s },
		Collect: func(t *model.TrialMetrics) *float64 { v := float64(t.WallTimeMs); return &v },
	},
	{
		Key:     "llmLatencyMs",
		Label:   "LLM Latency (ms)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.LLMLatencyMs },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.LLMLatencyMs = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.LLMLatencyMs) },
	},
	{
		Key:     "idleMs",
		Label:   "Idle Time (ms)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.IdleMs },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.IdleMs = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.IdleMs) },
	},
	{
		Key:     "ttftMs",
		Label:   "TTFT (ms)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.TTFTMs },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.TTFTMs = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.TTFTMs) },
	},
	{
		Key:     "llmTimeRatio",
		Label:   "LLM Time Ratio",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.LLMTimeRatio },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.LLMTimeRatio = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectFloat64(t.LLMTimeRatio) },
	},
	{
		Key:     "inputTokens",
		Label:   "Input Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.InputTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.InputTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.InputTokens) },
	},
	{
		Key:     "outputTokens",
		Label:   "Output Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.OutputTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.OutputTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.OutputTokens) },
	},
	{
		Key:     "totalTokens",
		Label:   "Total Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.TotalTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.TotalTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.TotalTokens) },
	},
	{
		Key:     "reasoningTokens",
		Label:   "Reasoning Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.ReasoningTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.ReasoningTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.ReasoningTokens) },
	},
	{
		Key:     "cacheReadTokens",
		Label:   "Cache Read Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.CacheReadTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.CacheReadTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.CacheReadTokens) },
	},
	{
		Key:     "cacheWriteTokens",
		Label:   "Cache Write Tokens",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.CacheWriteTokens },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.CacheWriteTokens = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt64(t.CacheWriteTokens) },
	},
	{
		Key:     "outputLength",
		Label:   "Output Length (chars)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.OutputLength },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.OutputLength = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectInt(t.OutputLength) },
	},
	{
		Key:     "tokensPerGeneration",
		Label:   "Tokens/Generation",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.TokensPerGen },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.TokensPerGen = s },
		Collect: func(t *model.TrialMetrics) *float64 {
			if t.TotalTokens == nil {
				return nil
			}
			genCount := 1
			if t.GenerationCount != nil && *t.GenerationCount > 0 {
				genCount = *t.GenerationCount
			} else if len(t.ToolCalls) > 0 {
				genCount = len(t.ToolCalls) + 1
			}
			v := float64(*t.TotalTokens) / float64(genCount)
			return &v
		},
	},
	{
		Key:     "estimatedCost",
		Label:   "Estimated Cost ($)",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.EstimatedCost },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.EstimatedCost = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectFloat64(t.EstimatedCost) },
	},
	{
		Key:     "accuracyScore",
		Label:   "Accuracy Score",
		Extract: func(m *model.MetricsSummary) *model.StatsSummary { return m.AccuracyScore },
		Set:     func(m *model.MetricsSummary, s *model.StatsSummary) { m.AccuracyScore = s },
		Collect: func(t *model.TrialMetrics) *float64 { return collectFloat64(t.AccuracyScore) },
	},
}
