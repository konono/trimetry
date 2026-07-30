package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type parsedOutput struct {
	Text   string
	Tokens *TokenInfo
	CostUSD *float64
	Steps  []StepDetail
	TTFTMs *int64
}

type rawExecResult struct {
	Stdout     string
	Stderr     string
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Err        error
	TimedOut   bool
	Timeout    time.Duration
}

func runCLI(ctx context.Context, args []string, workingDir string, ec ExecutionContext) *rawExecResult {
	timeout := time.Duration(ec.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	otelAttrs := fmt.Sprintf("benchmark.run_id=%s,benchmark.trial_id=%s,benchmark.scenario_id=%s,benchmark.trial_number=%d,benchmark.model_name=%s,benchmark.model_provider=%s",
		ec.BenchmarkRunID, ec.TrialID, ec.ScenarioID, ec.TrialNumber, ec.ModelName, ec.ModelProvider)

	cmd.Env = append(cmd.Environ(),
		fmt.Sprintf("OTEL_RESOURCE_ATTRIBUTES=%s", otelAttrs),
		fmt.Sprintf("BENCHMARK_RUN_ID=%s", ec.BenchmarkRunID),
		fmt.Sprintf("BENCHMARK_TRIAL_ID=%s", ec.TrialID),
		fmt.Sprintf("BENCHMARK_SCENARIO_ID=%s", ec.ScenarioID),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	err := cmd.Run()
	finishedAt := time.Now()

	result := &rawExecResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Timeout:    timeout,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			result.Err = fmt.Errorf("timeout after %v", timeout)
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Err = fmt.Errorf("exit code %d: %s", result.ExitCode, stderr.String())
		} else {
			result.Err = err
		}
	}

	return result
}

func toExecutionResult(raw *rawExecResult, parsed parsedOutput) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Output:     parsed.Text,
		StartedAt:  raw.StartedAt,
		FinishedAt: raw.FinishedAt,
		ExitCode:   raw.ExitCode,
		Stderr:     raw.Stderr,
		Tokens:     parsed.Tokens,
		CostUSD:    parsed.CostUSD,
		Steps:      parsed.Steps,
		TTFTMs:     parsed.TTFTMs,
	}

	if raw.Err != nil {
		if raw.TimedOut || raw.ExitCode != 0 {
			result.Error = raw.Err
			return result, nil
		}
		return result, fmt.Errorf("exec: %w", raw.Err)
	}

	return result, nil
}
