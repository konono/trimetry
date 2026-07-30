package metrics

import (
	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/model"
)

type Input struct {
	Result     *adapter.ExecutionResult
	StartedAt  int64
	FinishedAt int64
	Output     string
	Pricing    *model.ModelPricing
	RetryCount int
}

func BuildTrialMetrics(in Input) *model.TrialMetrics {
	wallTimeMs := in.FinishedAt - in.StartedAt // already in milliseconds
	if in.Result != nil && !in.Result.StartedAt.IsZero() && !in.Result.FinishedAt.IsZero() {
		wallTimeMs = in.Result.FinishedAt.Sub(in.Result.StartedAt).Milliseconds()
	}

	m := &model.TrialMetrics{
		WallTimeMs:       wallTimeMs,
		RetryCount:       in.RetryCount,
		TokenUsageSource: "unknown",
		CostSource:       "unknown",
	}

	if in.Result != nil && in.Result.Tokens != nil {
		t := in.Result.Tokens
		m.InputTokens = &t.Input
		m.OutputTokens = &t.Output
		m.TotalTokens = &t.Total
		m.TokenUsageSource = "provider"
		if t.Reasoning > 0 {
			m.ReasoningTokens = &t.Reasoning
		}
		if t.CacheRead > 0 {
			m.CacheReadTokens = &t.CacheRead
		}
		if t.CacheWrite > 0 {
			m.CacheWriteTokens = &t.CacheWrite
		}
	}

	if in.Result != nil && len(in.Result.Steps) > 0 {
		var llmInference int64
		var genCount int
		var toolCalls []model.ToolCall
		for _, s := range in.Result.Steps {
			switch s.Type {
			case adapter.StepTypeGeneration:
				llmInference += s.LLMInferenceMs
				genCount++
			case adapter.StepTypeTool:
				tc := model.ToolCall{
					Tool:       s.Name,
					Command:    extractCommand(s.Input),
					DurationMs: s.DurationMs,
				}
				if out, ok := s.Output.(string); ok && len(out) <= 200 {
					tc.Output = out
				}
				toolCalls = append(toolCalls, tc)
			}
		}
		if llmInference > 0 {
			m.LLMLatencyMs = &llmInference
		}
		if len(toolCalls) > 0 {
			m.ToolCalls = toolCalls
		}
		if genCount > 0 {
			m.GenerationCount = &genCount
		}
	}

	if in.Result != nil && in.Result.TTFTMs != nil {
		m.TTFTMs = in.Result.TTFTMs
	}

	if in.Output != "" {
		outLen := len(in.Output)
		m.OutputLength = &outLen
	}

	if m.LLMLatencyMs != nil && m.WallTimeMs > 0 {
		ratio := float64(*m.LLMLatencyMs) / float64(m.WallTimeMs)
		m.LLMTimeRatio = &ratio
		var toolMs int64
		for _, tc := range m.ToolCalls {
			toolMs += tc.DurationMs
		}
		idle := m.WallTimeMs - *m.LLMLatencyMs - toolMs
		if idle > 0 {
			m.IdleMs = &idle
		}
	}

	if in.Pricing != nil && m.InputTokens != nil && m.OutputTokens != nil {
		cost := float64(*m.InputTokens)/1_000_000*in.Pricing.InputPerMToken +
			float64(*m.OutputTokens)/1_000_000*in.Pricing.OutputPerMToken
		m.EstimatedCost = &cost
		m.CostSource = "estimated"
	}

	if in.Result != nil && in.Result.CostUSD != nil {
		m.EstimatedCost = in.Result.CostUSD
		m.CostSource = "provider"
	}

	return m
}

func extractCommand(input any) string {
	if input == nil {
		return ""
	}
	switch v := input.(type) {
	case string:
		return v
	case map[string]any:
		for _, key := range []string{"command", "pattern", "path", "file_path", "query"} {
			if cmd, ok := v[key].(string); ok && cmd != "" {
				return cmd
			}
		}
	}
	return ""
}
