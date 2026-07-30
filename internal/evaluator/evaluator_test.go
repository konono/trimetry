package evaluator

import (
	"testing"

	"github.com/konono/trimetry/internal/model"
)

func TestEvaluateAccuracy(t *testing.T) {
	tests := []struct {
		name           string
		expected       string
		output         string
		wantScore      *float64
		wantMethod     string
	}{
		{
			name:       "contains match",
			expected:   "sort",
			output:     "def sort(arr)",
			wantScore:  ptrFloat(1.0),
			wantMethod: "contains",
		},
		{
			name:       "no match",
			expected:   "sort",
			output:     "hello world",
			wantScore:  ptrFloat(0.0),
			wantMethod: "contains",
		},
		{
			name:       "empty expected returns nil",
			expected:   "",
			output:     "some output",
			wantScore:  nil,
			wantMethod: "",
		},
		{
			name:       "empty output with non-empty expected",
			expected:   "sort",
			output:     "",
			wantScore:  ptrFloat(0.0),
			wantMethod: "contains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenario := model.Scenario{ExpectedOutput: tt.expected}
			score, method, _ := EvaluateAccuracy(scenario, tt.output)

			if tt.wantScore == nil {
				if score != nil {
					t.Fatalf("expected nil score, got %v", *score)
				}
				return
			}
			if score == nil {
				t.Fatalf("expected score %v, got nil", *tt.wantScore)
			}
			if *score != *tt.wantScore {
				t.Errorf("score = %v, want %v", *score, *tt.wantScore)
			}
			if method != tt.wantMethod {
				t.Errorf("method = %q, want %q", method, tt.wantMethod)
			}
		})
	}
}

func TestRunEvaluations(t *testing.T) {
	tests := []struct {
		name           string
		trial          model.Trial
		accuracy       *float64
		accuracyReason string
		wantCount      int
		wantChecks     map[string]float64 // evaluatorName -> expected score
	}{
		{
			name: "completed trial with output and accuracy",
			trial: model.Trial{
				ExecutionStatus: model.ExecStatusCompleted,
				Output:          "def sort(arr): pass",
			},
			accuracy:       ptrFloat(1.0),
			accuracyReason: "output contains \"sort\"",
			wantCount:      3,
			wantChecks: map[string]float64{
				"completion": 1.0,
				"non_empty":  1.0,
				"accuracy":   1.0,
			},
		},
		{
			name: "failed trial with empty output no accuracy",
			trial: model.Trial{
				ExecutionStatus: model.ExecStatusFailed,
				Output:          "",
			},
			accuracy:  nil,
			wantCount: 2,
			wantChecks: map[string]float64{
				"completion": 0.0,
				"non_empty":  0.0,
			},
		},
		{
			name: "completed trial with empty output",
			trial: model.Trial{
				ExecutionStatus: model.ExecStatusCompleted,
				Output:          "",
			},
			accuracy:  nil,
			wantCount: 2,
			wantChecks: map[string]float64{
				"completion": 1.0,
				"non_empty":  0.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := RunEvaluations(tt.trial, tt.accuracy, tt.accuracyReason)
			if len(results) != tt.wantCount {
				t.Fatalf("got %d evaluations, want %d", len(results), tt.wantCount)
			}
			for _, r := range results {
				want, ok := tt.wantChecks[r.EvaluatorName]
				if !ok {
					t.Errorf("unexpected evaluation %q", r.EvaluatorName)
					continue
				}
				if r.Score == nil {
					t.Errorf("%s: score is nil", r.EvaluatorName)
					continue
				}
				if *r.Score != want {
					t.Errorf("%s: score = %v, want %v", r.EvaluatorName, *r.Score, want)
				}
			}
		})
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
