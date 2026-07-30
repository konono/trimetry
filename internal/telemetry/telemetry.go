package telemetry

import (
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/model"
)

type TrialContext struct {
	BenchmarkRunID  string
	BenchmarkName   string
	TrialID         string
	ScenarioID      string
	ScenarioVersion string
	TrialNumber     int
	ModelName       string
	ModelProvider   string
	ModelParameters map[string]any
	Input           string
}

type TrialResult struct {
	TrialID          string
	StartedAt        time.Time
	AdapterName      string
	ExecutionStatus  model.ExecutionStatus
	Output           string
	ModelName        string
	Metrics          *model.TrialMetrics
	Steps            []adapter.StepDetail
	Evaluations      []model.EvaluationResult
	EnrichmentDir    string
}

type EnvironmentInfo struct {
	HostName string
	HostArch string
	AISettings map[string]any
}

type Adapter interface {
	StartTrial(ctx TrialContext)
	FinishTrial(result TrialResult)
	Flush()
}

type reasoningInfo struct {
	Reasoning    string
	FinishReason string
	Timestamp    string
	AISettings   map[string]any
}

type enrichmentResult struct {
	Reasonings  []reasoningInfo
	Environment *EnvironmentInfo
}
