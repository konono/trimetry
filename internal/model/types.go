package model

import "time"

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
)

type ExecutionStatus string

const (
	ExecStatusPending   ExecutionStatus = "pending"
	ExecStatusRunning   ExecutionStatus = "running"
	ExecStatusCompleted ExecutionStatus = "completed"
	ExecStatusFailed    ExecutionStatus = "failed"
	ExecStatusTimeout   ExecutionStatus = "timeout"
	ExecStatusCancelled ExecutionStatus = "cancelled"
)

type ErrorType string

const (
	ErrApplication   ErrorType = "application"
	ErrTimeout       ErrorType = "timeout"
	ErrCancelled     ErrorType = "cancelled"
)

type BenchmarkRun struct {
	BenchmarkRunID string     `json:"benchmarkRunId"`
	Name           string     `json:"name"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	RunnerVersion  string     `json:"runnerVersion"`
	Environment    string     `json:"environment"`
	GitCommit      string     `json:"gitCommit"`
	ConfigHash     string     `json:"configHash"`
	Status         RunStatus  `json:"status"`
	Trials         []Trial    `json:"trials,omitempty"`
}

type Scenario struct {
	ScenarioID         string         `json:"scenarioId"         yaml:"id"`
	ScenarioVersion    string         `json:"scenarioVersion"    yaml:"version"`
	Name               string         `json:"name"               yaml:"name"`
	Input              string         `json:"input"              yaml:"input"`
	Metadata           map[string]any `json:"metadata,omitempty" yaml:"metadata"`
	TimeoutSeconds     int            `json:"timeoutSeconds"     yaml:"timeout_seconds"`
	ExpectedOutput     string         `json:"expectedOutput,omitempty" yaml:"expected_output"`
}

type ModelConfig struct {
	Name       string         `json:"name"       yaml:"name"`
	Provider   string         `json:"provider"   yaml:"provider"`
	Parameters map[string]any `json:"parameters" yaml:"parameters"`
	Pricing    *ModelPricing  `json:"pricing,omitempty" yaml:"pricing"`
}

type ModelPricing struct {
	InputPerMToken  float64 `json:"inputPerMToken"  yaml:"input_per_m_token"`
	OutputPerMToken float64 `json:"outputPerMToken" yaml:"output_per_m_token"`
}

type Trial struct {
	TrialID          string             `json:"trialId"`
	BenchmarkRunID   string             `json:"benchmarkRunId"`
	ScenarioID       string             `json:"scenarioId"`
	ScenarioVersion  string             `json:"scenarioVersion"`
	TrialNumber      int                `json:"trialNumber"`
	ModelName        string             `json:"modelName"`
	ModelProvider    string             `json:"modelProvider"`
	ModelParameters  map[string]any     `json:"modelParameters,omitempty"`
	StartedAt        time.Time          `json:"startedAt"`
	FinishedAt       *time.Time         `json:"finishedAt,omitempty"`
	ExecutionStatus  ExecutionStatus    `json:"executionStatus"`
	ErrorType        ErrorType          `json:"errorType,omitempty"`
	ErrorMessage     string             `json:"errorMessage,omitempty"`
	Output           string             `json:"output,omitempty"`
	Metrics          *TrialMetrics      `json:"metrics,omitempty"`
	Evaluations      []EvaluationResult `json:"evaluations,omitempty"`
}

type TrialMetrics struct {
	WallTimeMs       int64      `json:"wallTimeMs"`
	LLMLatencyMs     *int64     `json:"llmLatencyMs"`
	IdleMs           *int64     `json:"idleMs"`
	TTFTMs           *int64     `json:"ttftMs"`
	LLMTimeRatio     *float64   `json:"llmTimeRatio"`
	InputTokens      *int64     `json:"inputTokens"`
	OutputTokens     *int64     `json:"outputTokens"`
	TotalTokens      *int64     `json:"totalTokens"`
	ReasoningTokens  *int64     `json:"reasoningTokens,omitempty"`
	CacheReadTokens  *int64     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int64     `json:"cacheWriteTokens,omitempty"`
	OutputLength     *int       `json:"outputLength"`
	EstimatedCost    *float64   `json:"estimatedCost"`
	RetryCount       int        `json:"retryCount"`
	ToolCalls        []ToolCall `json:"toolCalls,omitempty"`
	GenerationCount  *int       `json:"generationCount,omitempty"`
	AccuracyScore    *float64   `json:"accuracyScore"`
	AccuracyMethod   string     `json:"accuracyMethod,omitempty"`
	AccuracyReason   string     `json:"accuracyReason,omitempty"`
	TokenUsageSource string     `json:"tokenUsageSource"`
	CostSource       string     `json:"costSource"`
}

type ToolCall struct {
	Tool       string `json:"tool"`
	Command    string `json:"command"`
	DurationMs int64  `json:"durationMs"`
	Output     string `json:"output,omitempty"`
}

type EvaluationResult struct {
	EvaluatorName    string         `json:"evaluatorName"`
	EvaluatorVersion string         `json:"evaluatorVersion"`
	Score            *float64       `json:"score,omitempty"`
	Label            string         `json:"label,omitempty"`
	Passed           *bool          `json:"passed,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ScenarioSummary struct {
	ScenarioID      string              `json:"scenarioId"`
	ScenarioVersion string              `json:"scenarioVersion"`
	ScenarioName    string              `json:"scenarioName,omitempty"`
	Input           string              `json:"input,omitempty"`
	ModelName       string              `json:"modelName"`
	ModelProvider   string              `json:"modelProvider"`
	TrialCount      int                 `json:"trialCount"`
	CompletedCount  int                 `json:"completedCount"`
	FailedCount     int                 `json:"failedCount"`
	TimeoutCount    int                 `json:"timeoutCount"`
	CancelledCount  int                 `json:"cancelledCount,omitempty"`
	FailureRate     float64             `json:"failureRate"`
	AccuracyRate    *float64            `json:"accuracyRate,omitempty"`
	AccuracyMethod  string              `json:"accuracyMethod,omitempty"`
	Metrics         *MetricsSummary `json:"metrics,omitempty"`
	OutputSamples   []OutputSample  `json:"outputSamples,omitempty"`
	ToolCalls       []ToolCall      `json:"toolCalls,omitempty"`
}

type OutputSample struct {
	TrialNumber int    `json:"trialNumber"`
	Output      string `json:"output"`
	Accurate    *bool  `json:"accurate,omitempty"`
}

type MetricsSummary struct {
	WallTimeMs    *StatsSummary `json:"wallTimeMs,omitempty"`
	LLMLatencyMs  *StatsSummary `json:"llmLatencyMs,omitempty"`
	IdleMs        *StatsSummary `json:"idleMs,omitempty"`
	TTFTMs        *StatsSummary `json:"ttftMs,omitempty"`
	LLMTimeRatio  *StatsSummary `json:"llmTimeRatio,omitempty"`
	InputTokens   *StatsSummary `json:"inputTokens,omitempty"`
	OutputTokens  *StatsSummary `json:"outputTokens,omitempty"`
	TotalTokens      *StatsSummary `json:"totalTokens,omitempty"`
	ReasoningTokens  *StatsSummary `json:"reasoningTokens,omitempty"`
	CacheReadTokens  *StatsSummary `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *StatsSummary `json:"cacheWriteTokens,omitempty"`
	OutputLength     *StatsSummary `json:"outputLength,omitempty"`
	TokensPerGen  *StatsSummary `json:"tokensPerGeneration,omitempty"`
	EstimatedCost *StatsSummary `json:"estimatedCost,omitempty"`
	AccuracyScore *StatsSummary `json:"accuracyScore,omitempty"`
}

type StatsSummary struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	Count  int     `json:"count"`
}

type RunManifest struct {
	BenchmarkRunID string         `json:"benchmarkRunId"`
	Name           string         `json:"name"`
	StartedAt      time.Time      `json:"startedAt"`
	RunnerVersion  string         `json:"runnerVersion"`
	Environment    string         `json:"environment"`
	GitCommit      string         `json:"gitCommit"`
	ConfigHash          string    `json:"configHash"`
	EffectiveConfigHash string    `json:"effectiveConfigHash,omitempty"`
	Scenarios      []Scenario     `json:"scenarios"`
	Models         []ModelConfig  `json:"models"`
	TrialsPerCombo int            `json:"trialsPerCombo"`
	Concurrency    int            `json:"concurrency"`
	Retries        int            `json:"retries"`
	Telemetry      map[string]any `json:"telemetry,omitempty"`
	Adapter        map[string]any `json:"adapter,omitempty"`
	DryRun         bool           `json:"dryRun,omitempty"`
}

type RunSummary struct {
	BenchmarkRunID string             `json:"benchmarkRunId"`
	Name           string             `json:"name"`
	Status         string             `json:"status"`
	StartedAt      time.Time          `json:"startedAt"`
	FinishedAt     *time.Time         `json:"finishedAt,omitempty"`
	Scenarios      []ScenarioSummary  `json:"scenarios"`
}
