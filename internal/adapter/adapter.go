package adapter

import (
	"context"
	"time"
)

type ExecutionContext struct {
	BenchmarkRunID  string
	ScenarioID      string
	ScenarioVersion string
	TrialID         string
	TrialNumber     int
	ModelName       string
	ModelProvider   string
	ModelParameters map[string]any
	TimeoutSeconds  int
}

type ExecutionResult struct {
	Output          string
	StartedAt       time.Time
	FinishedAt      time.Time
	ExitCode        int
	Stderr          string
	Error           error
	Tokens          *TokenInfo
	Steps           []StepDetail
	TTFTMs          *int64   // Time To First Token (ms from step_start to first text)
	CostUSD         *float64 // actual cost reported by provider (e.g. Claude)
}

type TokenInfo struct {
	Input      int64
	Output     int64
	Total      int64
	Reasoning  int64
	CacheRead  int64
	CacheWrite int64
}

type StepDetail struct {
	Type           string
	Name           string
	StartMs        int64
	EndMs          int64
	DurationMs     int64
	LLMInferenceMs int64
	ToolTimeMs     int64
	Status         string
	Reason         string
	Input          any
	Output         any
	Metadata       map[string]any
	Tokens         *TokenInfo
	Model          string
	ToolsCalled    []string

	// tool-specific
	CallID  string // LLM-generated tool call ID
	Title   string // tool display title (file path, etc.)
	Display *ToolDisplay

	// generation-specific
	TTFTMs *int64 // Time To First Token within this step
}

type ToolDisplay struct {
	Type       string `json:"type,omitempty"`
	Path       string `json:"path,omitempty"`
	LineStart  int    `json:"lineStart,omitempty"`
	LineEnd    int    `json:"lineEnd,omitempty"`
	TotalLines int    `json:"totalLines,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type ApplicationAdapter interface {
	Name() string
	Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error)
}

const (
	StepTypeGeneration = "generation"
	StepTypeTool       = "tool"
)
