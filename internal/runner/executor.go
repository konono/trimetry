package runner

import (
	"context"
	"sync"
	"time"

	"github.com/konono/trimetry/internal/adapter"
	"github.com/konono/trimetry/internal/evaluator"
	"github.com/konono/trimetry/internal/metrics"
	"github.com/konono/trimetry/internal/model"
	"github.com/konono/trimetry/internal/telemetry"
)

type TrialSpec struct {
	Trial    model.Trial
	Scenario model.Scenario
	Input    string
	Pricing  *model.ModelPricing
}

type Executor struct {
	Adapter          adapter.ApplicationAdapter
	Telemetry        telemetry.Adapter
	Concurrency      int
	Retries          int
	FlushOnTrialEnd  bool
	EnrichmentDir    string
	BenchmarkName    string
	OnTrialComplete  func(model.Trial)
	OnRetry          func(trialID string, attempt, maxRetries int)
}

func (e *Executor) RunTrials(ctx context.Context, specs []TrialSpec) []model.Trial {
	results := make([]model.Trial, len(specs))
	sem := make(chan struct{}, e.Concurrency)
	var wg sync.WaitGroup

	for i, spec := range specs {
		wg.Add(1)
		go func(idx int, s TrialSpec) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				t := s.Trial
				t.ExecutionStatus = model.ExecStatusCancelled
				t.ErrorType = model.ErrCancelled
				t.ErrorMessage = "context cancelled"
				results[idx] = t
				if e.OnTrialComplete != nil {
					e.OnTrialComplete(t)
				}
				return
			}
			defer func() { <-sem }()

			results[idx] = e.runTrial(ctx, s)
		}(i, spec)
	}

	wg.Wait()
	return results
}

func (e *Executor) runTrial(ctx context.Context, spec TrialSpec) model.Trial {
	trial := spec.Trial
	trial.ExecutionStatus = model.ExecStatusRunning
	trial.StartedAt = time.Now()

	tc, ec := buildContexts(trial, spec.Input, spec.Scenario.TimeoutSeconds, e.BenchmarkName)
	e.Telemetry.StartTrial(tc)

	var result *adapter.ExecutionResult
	var execErr error
	retryCount := 0

	for attempt := 0; attempt <= e.Retries; attempt++ {
		if attempt > 0 {
			retryCount++
			if e.OnRetry != nil {
				e.OnRetry(trial.TrialID, attempt, e.Retries)
			}
		}

		result, execErr = e.Adapter.Execute(ctx, spec.Input, ec)
		if execErr == nil && result != nil && result.Error == nil {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}

	now := time.Now()
	trial.FinishedAt = &now

	applyOutcome(&trial, result, execErr)

	trial.Metrics = metrics.BuildTrialMetrics(metrics.Input{
		Result:     result,
		StartedAt:  trial.StartedAt.UnixMilli(),
		FinishedAt: trial.FinishedAt.UnixMilli(),
		Output:     trial.Output,
		Pricing:    spec.Pricing,
		RetryCount: retryCount,
	})

	accuracy, method, reason := evaluator.EvaluateAccuracy(spec.Scenario, trial.Output)
	if accuracy != nil {
		trial.Metrics.AccuracyScore = accuracy
		trial.Metrics.AccuracyMethod = method
		trial.Metrics.AccuracyReason = reason
	}

	trial.Evaluations = evaluator.RunEvaluations(trial, accuracy, reason)

	var steps []adapter.StepDetail
	if result != nil {
		steps = result.Steps
	}

	e.Telemetry.FinishTrial(telemetry.TrialResult{
		TrialID:          trial.TrialID,
		StartedAt:        trial.StartedAt,
		AdapterName:      e.Adapter.Name(),
		ExecutionStatus:  trial.ExecutionStatus,
		Output:           trial.Output,
		ModelName:        trial.ModelName,
		Metrics:          trial.Metrics,
		Steps:            steps,
		Evaluations:      trial.Evaluations,
		EnrichmentDir:    e.EnrichmentDir,
	})

	if e.FlushOnTrialEnd {
		e.Telemetry.Flush()
	}

	if e.OnTrialComplete != nil {
		e.OnTrialComplete(trial)
	}

	return trial
}

func applyOutcome(trial *model.Trial, result *adapter.ExecutionResult, execErr error) {
	if execErr != nil {
		trial.ExecutionStatus = model.ExecStatusFailed
		trial.ErrorType = model.ErrApplication
		trial.ErrorMessage = execErr.Error()
	} else if result == nil {
		trial.ExecutionStatus = model.ExecStatusFailed
		trial.ErrorType = model.ErrApplication
		trial.ErrorMessage = "adapter returned nil result"
	} else if result.Error != nil {
		if result.ExitCode == -1 {
			trial.ExecutionStatus = model.ExecStatusTimeout
			trial.ErrorType = model.ErrTimeout
		} else {
			trial.ExecutionStatus = model.ExecStatusFailed
			trial.ErrorType = model.ErrApplication
		}
		trial.ErrorMessage = result.Error.Error()
	} else {
		trial.ExecutionStatus = model.ExecStatusCompleted
		trial.Output = result.Output
	}
}

func buildContexts(trial model.Trial, input string, timeoutSeconds int, benchmarkName string) (telemetry.TrialContext, adapter.ExecutionContext) {
	tc := telemetry.TrialContext{
		BenchmarkRunID:  trial.BenchmarkRunID,
		BenchmarkName:   benchmarkName,
		TrialID:         trial.TrialID,
		ScenarioID:      trial.ScenarioID,
		ScenarioVersion: trial.ScenarioVersion,
		TrialNumber:     trial.TrialNumber,
		ModelName:       trial.ModelName,
		ModelProvider:   trial.ModelProvider,
		ModelParameters: trial.ModelParameters,
		Input:           input,
	}
	ec := adapter.ExecutionContext{
		BenchmarkRunID:  trial.BenchmarkRunID,
		ScenarioID:      trial.ScenarioID,
		ScenarioVersion: trial.ScenarioVersion,
		TrialID:         trial.TrialID,
		TrialNumber:     trial.TrialNumber,
		ModelName:       trial.ModelName,
		ModelProvider:   trial.ModelProvider,
		ModelParameters: trial.ModelParameters,
		TimeoutSeconds:  timeoutSeconds,
	}
	return tc, ec
}
