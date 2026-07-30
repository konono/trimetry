package adapter

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

type FakeAdapter struct {
	Latency        time.Duration
	LatencyJitter  time.Duration
	FailRate       float64
	TimeoutRate    float64
	InputTokens    int64
	OutputTokens   int64
}

func NewFakeAdapter() *FakeAdapter {
	return &FakeAdapter{
		Latency:         200 * time.Millisecond,
		LatencyJitter:   50 * time.Millisecond,
		InputTokens:     150,
		OutputTokens:    80,
	}
}

func (a *FakeAdapter) Name() string { return "fake" }

func (a *FakeAdapter) Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error) {
	jitter := time.Duration(rand.Int64N(int64(a.LatencyJitter)))
	delay := a.Latency + jitter

	startedAt := time.Now()

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return &ExecutionResult{
			StartedAt:  startedAt,
			FinishedAt: time.Now(),
			ExitCode:   -1,
			Error:      fmt.Errorf("timeout"),
		}, nil
	}

	finishedAt := time.Now()

	if a.TimeoutRate > 0 && rand.Float64() < a.TimeoutRate {
		return &ExecutionResult{
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			ExitCode:   -1,
			Error:      fmt.Errorf("simulated timeout"),
		}, nil
	}

	if a.FailRate > 0 && rand.Float64() < a.FailRate {
		return &ExecutionResult{
			Output:     "",
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			ExitCode:   1,
			Error:      fmt.Errorf("simulated error"),
		}, nil
	}

	inTok := a.InputTokens
	outTok := a.OutputTokens
	totalTok := inTok + outTok

	genStart := startedAt.UnixMilli()
	genEnd := finishedAt.UnixMilli()
	toolStart := genStart + (genEnd-genStart)/3
	toolEnd := toolStart + 30

	steps := []StepDetail{
		{Type: StepTypeGeneration, Name: "llm (tool-calls)", StartMs: genStart, EndMs: toolStart, DurationMs: toolStart - genStart, LLMInferenceMs: toolStart - genStart - 30, Status: "completed", Reason: "tool-calls"},
		{Type: StepTypeTool, Name: "glob", StartMs: toolStart, EndMs: toolEnd, DurationMs: 30, Status: "completed"},
		{Type: StepTypeGeneration, Name: "llm (stop)", StartMs: toolEnd, EndMs: genEnd, DurationMs: genEnd - toolEnd, LLMInferenceMs: genEnd - toolEnd, Status: "completed", Reason: "stop"},
	}

	ttft := int64(50)

	return &ExecutionResult{
		Output:          fmt.Sprintf("Fake response to: %s (trial=%s)", input, ec.TrialID),
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		ExitCode:        0,
		Tokens:          &TokenInfo{Input: inTok, Output: outTok, Total: totalTok},
		Steps:           steps,
		TTFTMs:          &ttft,
	}, nil
}

