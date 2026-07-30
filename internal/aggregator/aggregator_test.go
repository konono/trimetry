package aggregator

import (
	"math"
	"testing"

	"github.com/konono/trimetry/internal/model"
)

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil)
	if s.TrialCount != 0 {
		t.Errorf("expected 0 trials, got %d", s.TrialCount)
	}
}

func TestSummarizeBasic(t *testing.T) {
	trials := []model.Trial{
		makeTrial(model.ExecStatusCompleted, 100, 50, 30, 80),
		makeTrial(model.ExecStatusCompleted, 200, 60, 40, 100),
		makeTrial(model.ExecStatusFailed, 150, 0, 0, 0),
		makeTrial(model.ExecStatusTimeout, 300, 0, 0, 0),
		makeTrial(model.ExecStatusCompleted, 250, 70, 50, 120),
	}

	s := Summarize(trials)

	if s.TrialCount != 5 {
		t.Errorf("expected 5 trials, got %d", s.TrialCount)
	}
	if s.CompletedCount != 3 {
		t.Errorf("expected 3 completed, got %d", s.CompletedCount)
	}
	if s.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", s.FailedCount)
	}
	if s.TimeoutCount != 1 {
		t.Errorf("expected 1 timeout, got %d", s.TimeoutCount)
	}
	if math.Abs(s.FailureRate-0.4) > 0.01 {
		t.Errorf("expected failure rate 0.4, got %f", s.FailureRate)
	}

	if s.Metrics == nil {
		t.Fatal("expected metrics")
	}
	if s.Metrics.WallTimeMs == nil {
		t.Fatal("expected wall time stats")
	}
	if s.Metrics.WallTimeMs.Count != 5 {
		t.Errorf("expected 5 wall time samples, got %d", s.Metrics.WallTimeMs.Count)
	}
	if s.Metrics.WallTimeMs.Min != 100 {
		t.Errorf("expected min 100, got %f", s.Metrics.WallTimeMs.Min)
	}
	if s.Metrics.WallTimeMs.Max != 300 {
		t.Errorf("expected max 300, got %f", s.Metrics.WallTimeMs.Max)
	}
}

func TestNullTokenHandling(t *testing.T) {
	trials := []model.Trial{
		{
			ScenarioID:      "s1",
			ModelName:       "m1",
			ExecutionStatus: model.ExecStatusCompleted,
			Metrics: &model.TrialMetrics{
				WallTimeMs:       100,
				TokenUsageSource: "unknown",
				CostSource:       "unknown",
			},
		},
	}

	s := Summarize(trials)

	if s.Metrics.InputTokens != nil {
		t.Error("expected nil input tokens when data is unknown")
	}
	if s.Metrics.OutputTokens != nil {
		t.Error("expected nil output tokens when data is unknown")
	}
}

func TestPercentile(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

	tests := []struct {
		p    float64
		want float64
	}{
		{50, 55},
		{90, 91},
		{95, 95.5},
		{0, 10},
		{100, 100},
	}

	for _, tt := range tests {
		got := percentile(values, tt.p)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("percentile(%v, %v) = %v, want %v", values, tt.p, got, tt.want)
		}
	}
}

func TestSummarizeCancelled(t *testing.T) {
	trials := []model.Trial{
		makeTrial(model.ExecStatusCompleted, 100, 50, 30, 80),
		{
			ScenarioID:      "s1",
			ScenarioVersion: "1",
			ModelName:       "m1",
			ModelProvider:    "p1",
			TrialNumber:     2,
			ExecutionStatus: model.ExecStatusCancelled,
			ErrorType:       model.ErrCancelled,
		},
	}
	s := Summarize(trials)
	if s.CancelledCount != 1 {
		t.Errorf("CancelledCount = %d, want 1", s.CancelledCount)
	}
	if s.CompletedCount != 1 {
		t.Errorf("CompletedCount = %d, want 1", s.CompletedCount)
	}
	// FailureRate should include cancelled: 1/2 = 0.5
	if s.FailureRate != 0.5 {
		t.Errorf("FailureRate = %f, want 0.5", s.FailureRate)
	}
}

func makeTrial(status model.ExecutionStatus, wallMs int64, inTok, outTok, totalTok int64) model.Trial {
	var inP, outP, totalP *int64
	if inTok > 0 {
		inP = &inTok
	}
	if outTok > 0 {
		outP = &outTok
	}
	if totalTok > 0 {
		totalP = &totalTok
	}

	return model.Trial{
		ScenarioID:      "s1",
		ModelName:       "m1",
		ExecutionStatus: status,
		Metrics: &model.TrialMetrics{
			WallTimeMs:       wallMs,
			InputTokens:      inP,
			OutputTokens:     outP,
			TotalTokens:      totalP,
			TokenUsageSource: "provider",
			CostSource:       "unknown",
		},
	}
}
