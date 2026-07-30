package metrics

import (
	"testing"

	"github.com/konono/trimetry/internal/model"
)

func TestFieldsCompleteness(t *testing.T) {
	if len(Fields) != 15 {
		t.Fatalf("expected 15 fields, got %d", len(Fields))
	}

	seen := make(map[string]bool)
	for _, f := range Fields {
		if seen[f.Key] {
			t.Errorf("duplicate field key: %s", f.Key)
		}
		seen[f.Key] = true
	}
}

func TestFieldsExtractSet(t *testing.T) {
	for _, f := range Fields {
		t.Run(f.Key, func(t *testing.T) {
			summary := &model.MetricsSummary{}
			statsValue := &model.StatsSummary{Mean: 42, Median: 40, StdDev: 5, Min: 30, Max: 60, P90: 55, P95: 58, Count: 10}

			f.Set(summary, statsValue)
			got := f.Extract(summary)

			if got != statsValue {
				t.Errorf("Extract after Set: got %v, want %v", got, statsValue)
			}
		})
	}
}

func TestTokensPerGenerationCollect(t *testing.T) {
	// Find the tokensPerGeneration field.
	var tpgField Field
	for _, f := range Fields {
		if f.Key == "tokensPerGeneration" {
			tpgField = f
			break
		}
	}
	if tpgField.Key == "" {
		t.Fatal("tokensPerGeneration field not found")
	}

	tests := []struct {
		name    string
		metrics model.TrialMetrics
		want    *float64
	}{
		{
			name: "with GenerationCount",
			metrics: model.TrialMetrics{
				TotalTokens:     ptrInt64(300),
				GenerationCount: ptrInt(3),
			},
			want: ptrFloat(100.0),
		},
		{
			name: "fallback to ToolCalls count",
			metrics: model.TrialMetrics{
				TotalTokens: ptrInt64(300),
				ToolCalls: []model.ToolCall{
					{Tool: "a", DurationMs: 10},
					{Tool: "b", DurationMs: 20},
				},
			},
			want: ptrFloat(100.0),
		},
		{
			name: "no GenerationCount no ToolCalls",
			metrics: model.TrialMetrics{
				TotalTokens: ptrInt64(300),
			},
			want: ptrFloat(300.0),
		},
		{
			name: "nil TotalTokens returns nil",
			metrics: model.TrialMetrics{
				GenerationCount: ptrInt(3),
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tpgField.Collect(&tt.metrics)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %v, got nil", *tt.want)
			}
			if *got != *tt.want {
				t.Errorf("got %v, want %v", *got, *tt.want)
			}
		})
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}

func ptrInt64(v int64) *int64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}
