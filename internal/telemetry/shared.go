package telemetry

import (
	"github.com/konono/trimetry/internal/adapter"
)

func mergeEnrichmentIntoSteps(steps []adapter.StepDetail, enrichment enrichmentResult, defaultModel string) []adapter.StepDetail {
	out := make([]adapter.StepDetail, len(steps))
	copy(out, steps)

	genIndex := 0
	for i := range out {
		if out[i].Type != adapter.StepTypeGeneration {
			continue
		}
		if out[i].Model == "" {
			out[i].Model = defaultModel
		}
		if genIndex < len(enrichment.Reasonings) {
			r := enrichment.Reasonings[genIndex]
			out[i].Output = map[string]any{
				"text":         out[i].Output,
				"reasoning":    r.Reasoning,
				"finishReason": r.FinishReason,
			}
			if len(r.AISettings) > 0 {
				if out[i].Metadata == nil {
					out[i].Metadata = make(map[string]any)
				}
				out[i].Metadata["aiSettings"] = r.AISettings
			}
		}
		genIndex++
	}
	return out
}

func prepareEnrichedSteps(result TrialResult) ([]adapter.StepDetail, enrichmentResult) {
	var enrichment enrichmentResult
	if result.AdapterName == "opencode" && result.EnrichmentDir != "" {
		enrichment = collectFileEnrichment(result.EnrichmentDir, result.TrialID)
	}
	steps := mergeEnrichmentIntoSteps(result.Steps, enrichment, result.ModelName)
	return steps, enrichment
}

func buildTrialOutput(result TrialResult) map[string]any {
	output := map[string]any{
		"executionStatus": result.ExecutionStatus,
		"text":            result.Output,
	}
	if result.Metrics != nil {
		output["wallTimeMs"] = result.Metrics.WallTimeMs
		if result.Metrics.TotalTokens != nil {
			output["totalTokens"] = *result.Metrics.TotalTokens
		}
		if result.Metrics.AccuracyScore != nil {
			output["accuracyScore"] = *result.Metrics.AccuracyScore
		}
		if result.Metrics.EstimatedCost != nil {
			output["estimatedCost"] = *result.Metrics.EstimatedCost
		}
	}
	return output
}
