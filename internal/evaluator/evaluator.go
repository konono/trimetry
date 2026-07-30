package evaluator

import (
	"fmt"
	"strings"

	"github.com/konono/trimetry/internal/model"
)

const Version = "1.0.0"

func EvaluateAccuracy(scenario model.Scenario, output string) (*float64, string, string) {
	if scenario.ExpectedOutput != "" {
		score := 0.0
		reason := fmt.Sprintf("output does not contain %q", scenario.ExpectedOutput)
		if strings.Contains(output, scenario.ExpectedOutput) {
			score = 1.0
			reason = fmt.Sprintf("output contains %q", scenario.ExpectedOutput)
		}
		return &score, "contains", reason
	}
	return nil, "", ""
}

func RunEvaluations(trial model.Trial, accuracy *float64, accuracyReason string) []model.EvaluationResult {
	completed := trial.ExecutionStatus == model.ExecStatusCompleted
	completionScore := 0.0
	if completed {
		completionScore = 1.0
	}

	output := strings.TrimSpace(trial.Output)
	nonEmpty := len(output) > 0
	nonEmptyScore := 0.0
	if nonEmpty {
		nonEmptyScore = 1.0
	}

	evals := []model.EvaluationResult{
		{
			EvaluatorName:    "completion",
			EvaluatorVersion: Version,
			Score:            &completionScore,
			Passed:           &completed,
			Reason:           fmt.Sprintf("execution status: %s", trial.ExecutionStatus),
		},
		{
			EvaluatorName:    "non_empty",
			EvaluatorVersion: Version,
			Score:            &nonEmptyScore,
			Passed:           &nonEmpty,
			Reason:           fmt.Sprintf("output length: %d", len(output)),
		},
	}

	if accuracy != nil {
		passed := *accuracy == 1.0
		evals = append(evals, model.EvaluationResult{
			EvaluatorName:    "accuracy",
			EvaluatorVersion: Version,
			Score:            accuracy,
			Passed:           &passed,
			Reason:           accuracyReason,
		})
	}

	return evals
}
