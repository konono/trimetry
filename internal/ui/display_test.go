package ui

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/konono/trimetry/internal/model"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0ms"},
		{500 * time.Millisecond, "500ms"},
		{999 * time.Millisecond, "999ms"},
		{1500 * time.Millisecond, "1.5s"},
		{59900 * time.Millisecond, "59.9s"},
		{60 * time.Second, "1m00s"},
		{150 * time.Second, "2m30s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.input)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatMs(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.05, "0.05ms"},
		{0.3, "0.30ms"},
		{0.99, "0.99ms"},
		{1.0, "1.0ms"},
		{5.23, "5.2ms"},
		{9.99, "10.0ms"},
		{10.0, "10ms"},
		{500.0, "500ms"},
		{1500.0, "1.5s"},
	}
	for _, tt := range tests {
		got := formatMs(tt.input)
		if got != tt.want {
			t.Errorf("formatMs(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{-500, "-500"},
		{-5000, "-5,000"},
		{-1234567, "-1,234,567"},
		{math.MaxInt64, "9,223,372,036,854,775,807"},
	}
	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"▲ trimetry", 10},
		{"model × scenario", 16},
		{"╭──╮", 4},
	}
	for _, tt := range tests {
		got := displayWidth(tt.input)
		if got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"toolong", 3, "toolong"},
		{"▲", 5, "▲    "},
	}
	for _, tt := range tests {
		got := pad(tt.input, tt.width)
		if got != tt.want {
			t.Errorf("pad(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
		}
	}
}

func newTestDisplay(t *testing.T, scenarios []model.Scenario, models []model.ModelConfig, trialsPerCombo int, buf *bytes.Buffer) *Display {
	t.Helper()
	run := &model.BenchmarkRun{
		BenchmarkRunID: "run-1",
		Name:           "test",
		StartedAt:      time.Now(),
		RunnerVersion:  "dev",
	}
	return NewDisplay(run, scenarios, models, trialsPerCombo, 1, buf, false)
}

func TestOnTrialComplete(t *testing.T) {
	scenarios := []model.Scenario{
		{ScenarioID: "s1", ScenarioVersion: "1", Name: "Scenario1", Input: "test"},
	}
	models := []model.ModelConfig{
		{Name: "m1", Provider: "fake"},
	}

	var buf bytes.Buffer
	d := newTestDisplay(t, scenarios, models, 4, &buf)
	buf.Reset()

	statuses := []model.ExecutionStatus{
		model.ExecStatusCompleted,
		model.ExecStatusFailed,
		model.ExecStatusTimeout,
		model.ExecStatusCancelled,
	}

	for i, status := range statuses {
		trial := model.Trial{
			ScenarioID:      "s1",
			ModelName:       "m1",
			TrialNumber:     i + 1,
			ExecutionStatus: status,
		}
		d.OnTrialComplete(trial)
	}

	combo := d.combos[0]
	if combo.Completed != 1 {
		t.Errorf("Completed = %d, want 1", combo.Completed)
	}
	if combo.Failed != 1 {
		t.Errorf("Failed = %d, want 1", combo.Failed)
	}
	if combo.Timeout != 1 {
		t.Errorf("Timeout = %d, want 1", combo.Timeout)
	}
	if combo.Cancelled != 1 {
		t.Errorf("Cancelled = %d, want 1", combo.Cancelled)
	}
	if combo.done() != 4 {
		t.Errorf("done() = %d, want 4", combo.done())
	}
	if d.completed != 4 {
		t.Errorf("completed = %d, want 4", d.completed)
	}
}

func TestOnTrialCompletePlainOutput(t *testing.T) {
	scenarios := []model.Scenario{
		{ScenarioID: "s1", ScenarioVersion: "1", Name: "Scenario1", Input: "test"},
	}
	models := []model.ModelConfig{
		{Name: "m1", Provider: "fake"},
	}

	var buf bytes.Buffer
	d := newTestDisplay(t, scenarios, models, 2, &buf)
	buf.Reset()

	d.OnTrialComplete(model.Trial{
		ScenarioID:      "s1",
		ModelName:       "m1",
		TrialNumber:     1,
		ExecutionStatus: model.ExecStatusCompleted,
	})

	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("expected ✓ in output, got %q", output)
	}
	if !strings.Contains(output, "[trimetry]") {
		t.Errorf("expected [trimetry] prefix in plain mode, got %q", output)
	}
	if !strings.Contains(output, "Scenario1") {
		t.Errorf("expected friendly scenario name 'Scenario1' in plain output, got %q", output)
	}
}

func TestOnRetry(t *testing.T) {
	scenarios := []model.Scenario{
		{ScenarioID: "s1", ScenarioVersion: "1", Name: "S1", Input: "test"},
	}
	models := []model.ModelConfig{
		{Name: "m1", Provider: "fake"},
	}

	var buf bytes.Buffer
	d := newTestDisplay(t, scenarios, models, 1, &buf)
	buf.Reset()

	d.OnRetry("trial-123", 1, 3)

	output := buf.String()
	if !strings.Contains(output, "retry 1/3") {
		t.Errorf("expected retry message, got %q", output)
	}
	if !strings.Contains(output, "trial-123") {
		t.Errorf("expected trial ID in retry message, got %q", output)
	}
}

func TestHeaderShowsConcurrency(t *testing.T) {
	run := &model.BenchmarkRun{
		BenchmarkRunID: "run-1",
		Name:           "test",
		StartedAt:      time.Now(),
		RunnerVersion:  "dev",
	}
	scenarios := []model.Scenario{
		{ScenarioID: "s1", ScenarioVersion: "1", Name: "S1", Input: "test"},
	}
	models := []model.ModelConfig{
		{Name: "m1", Provider: "fake"},
	}

	var buf bytes.Buffer
	NewDisplay(run, scenarios, models, 3, 4, &buf, false)

	output := buf.String()
	if !strings.Contains(output, "concurrency 4") {
		t.Errorf("expected 'concurrency 4' in header, got %q", output)
	}
}

func TestStatusIcon(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(t, []model.Scenario{{ScenarioID: "s1", ScenarioVersion: "1", Input: "x"}}, []model.ModelConfig{{Name: "m1", Provider: "fake"}}, 1, &buf)

	tests := []struct {
		status model.ExecutionStatus
		want   string
	}{
		{model.ExecStatusCompleted, "✓"},
		{model.ExecStatusFailed, "✗"},
		{model.ExecStatusTimeout, "⏱"},
		{model.ExecStatusCancelled, "⊘"},
	}
	for _, tt := range tests {
		got := d.statusIcon(tt.status)
		if got != tt.want {
			t.Errorf("statusIcon(%s) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
