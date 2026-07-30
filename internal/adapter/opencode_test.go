package adapter

import (
	"strings"
	"testing"
)

func TestParseOpencodeJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		// assertions
		wantText       string
		wantInputTok   int64
		wantOutputTok  int64
		wantTotalTok   int64
		wantStepCount  int
		wantStepTypes  []string // StepTypeGeneration or StepTypeTool
		wantTTFTNonNil bool
	}{
		{
			name: "full generation and tool cycle",
			raw: joinLines(
				`{"type":"step_start","timestamp":1000000,"sessionID":"sess-1","part":{}}`,
				`{"type":"text","timestamp":1000100,"part":{"type":"text","text":"Hello "}}`,
				`{"type":"text","timestamp":1000200,"part":{"type":"text","text":"world"}}`,
				`{"type":"tool_use","timestamp":1000300,"part":{"type":"tool_use","tool":"Read","name":"Read","callID":"call-1","state":{"status":"completed","input":{"file_path":"/tmp/test.go"},"output":"file contents","time":{"start":1000300,"end":1000500}}}}`,
				`{"type":"step_finish","timestamp":1001000,"part":{"type":"step_finish","reason":"tool_use","time":{"start":1000000,"end":1001000},"tokens":{"total":230,"input":150,"output":80,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			),
			wantText:       "Hello world",
			wantInputTok:   150,
			wantOutputTok:  80,
			wantTotalTok:   230,
			wantStepCount:  2, // 1 generation + 1 tool
			wantStepTypes:  []string{StepTypeGeneration, StepTypeTool},
			wantTTFTNonNil: false, // TTFT requires Part.Time on text events; our fixture does not set it
		},
		{
			name: "TTFT captured from text event with time",
			raw: joinLines(
				`{"type":"step_start","timestamp":2000000,"sessionID":"sess-2","part":{}}`,
				`{"type":"text","timestamp":2000050,"part":{"type":"text","text":"Hi","time":{"start":2000050,"end":2000060}}}`,
				`{"type":"step_finish","timestamp":2001000,"part":{"type":"step_finish","reason":"stop","time":{"start":2000000,"end":2001000},"tokens":{"total":50,"input":30,"output":20,"reasoning":0}}}`,
			),
			wantText:       "Hi",
			wantInputTok:   30,
			wantOutputTok:  20,
			wantTotalTok:   50,
			wantStepCount:  1, // 1 generation only
			wantStepTypes:  []string{StepTypeGeneration},
			wantTTFTNonNil: true,
		},
		{
			name:          "empty input",
			raw:           "",
			wantText:      "",
			wantStepCount: 0,
		},
		{
			name: "text event without step_finish",
			raw: joinLines(
				`{"type":"text","timestamp":3000100,"part":{"type":"text","text":"orphan text"}}`,
			),
			wantText:      "orphan text",
			wantStepCount: 0, // no step_finish means no steps emitted
		},
		{
			name: "token accumulation across two steps",
			raw: joinLines(
				`{"type":"step_start","timestamp":4000000,"sessionID":"sess-3","part":{}}`,
				`{"type":"text","timestamp":4000100,"part":{"type":"text","text":"first "}}`,
				`{"type":"step_finish","timestamp":4001000,"part":{"type":"step_finish","reason":"tool_use","time":{"start":4000000,"end":4001000},"tokens":{"total":100,"input":60,"output":40,"reasoning":0,"cache":{"read":5,"write":3}}}}`,
				`{"type":"step_start","timestamp":4002000,"sessionID":"sess-3","part":{}}`,
				`{"type":"text","timestamp":4002100,"part":{"type":"text","text":"second"}}`,
				`{"type":"step_finish","timestamp":4003000,"part":{"type":"step_finish","reason":"stop","time":{"start":4002000,"end":4003000},"tokens":{"total":80,"input":50,"output":30,"reasoning":0,"cache":{"read":2,"write":1}}}}`,
			),
			wantText:      "first second",
			wantInputTok:  110, // 60 + 50
			wantOutputTok: 70,  // 40 + 30
			wantTotalTok:  180, // 100 + 80
			wantStepCount: 2,   // 2 generations
			wantStepTypes: []string{StepTypeGeneration, StepTypeGeneration},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOpencodeJSON(tt.raw)

			if got.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", got.Text, tt.wantText)
			}

			if tt.wantStepCount == 0 && tt.wantInputTok == 0 {
				// nothing more to check for empty/no-step cases
				if len(got.Steps) != 0 {
					t.Errorf("Steps count = %d, want 0", len(got.Steps))
				}
				return
			}

			if got.Tokens == nil {
				t.Fatalf("Tokens is nil, want non-nil")
			}
			if got.Tokens.Input != tt.wantInputTok {
				t.Errorf("Tokens.Input = %d, want %d", got.Tokens.Input, tt.wantInputTok)
			}
			if got.Tokens.Output != tt.wantOutputTok {
				t.Errorf("Tokens.Output = %d, want %d", got.Tokens.Output, tt.wantOutputTok)
			}
			if got.Tokens.Total != tt.wantTotalTok {
				t.Errorf("Tokens.Total = %d, want %d", got.Tokens.Total, tt.wantTotalTok)
			}

			if len(got.Steps) != tt.wantStepCount {
				t.Fatalf("Steps count = %d, want %d", len(got.Steps), tt.wantStepCount)
			}

			if tt.wantStepTypes != nil {
				for i, wantType := range tt.wantStepTypes {
					if got.Steps[i].Type != wantType {
						t.Errorf("Steps[%d].Type = %q, want %q", i, got.Steps[i].Type, wantType)
					}
				}
			}

			if tt.wantTTFTNonNil && got.TTFTMs == nil {
				t.Errorf("TTFTMs is nil, want non-nil")
			}
			if !tt.wantTTFTNonNil && tt.wantStepCount > 0 {
				// not asserting nil here since it depends on fixture
			}
		})
	}
}

func TestParseOpencodeJSON_ToolStepDetails(t *testing.T) {
	raw := joinLines(
		`{"type":"step_start","timestamp":5000000,"sessionID":"sess-5","part":{}}`,
		`{"type":"tool_use","timestamp":5000300,"part":{"type":"tool_use","tool":"Read","name":"Read","callID":"call-1","state":{"status":"completed","input":{"file_path":"/tmp/test.go"},"output":"file contents","time":{"start":5000300,"end":5000500}}}}`,
		`{"type":"step_finish","timestamp":5001000,"part":{"type":"step_finish","reason":"tool_use","time":{"start":5000000,"end":5001000},"tokens":{"total":100,"input":60,"output":40,"reasoning":0}}}`,
	)

	got := parseOpencodeJSON(raw)

	if len(got.Steps) != 2 {
		t.Fatalf("Steps count = %d, want 2", len(got.Steps))
	}

	gen := got.Steps[0]
	if gen.Type != StepTypeGeneration {
		t.Errorf("Steps[0].Type = %q, want %q", gen.Type, StepTypeGeneration)
	}
	if gen.Reason != "tool_use" {
		t.Errorf("Steps[0].Reason = %q, want %q", gen.Reason, "tool_use")
	}
	if len(gen.ToolsCalled) != 1 || gen.ToolsCalled[0] != "Read" {
		t.Errorf("Steps[0].ToolsCalled = %v, want [Read]", gen.ToolsCalled)
	}

	tool := got.Steps[1]
	if tool.Type != StepTypeTool {
		t.Errorf("Steps[1].Type = %q, want %q", tool.Type, StepTypeTool)
	}
	if tool.Name != "Read" {
		t.Errorf("Steps[1].Name = %q, want %q", tool.Name, "Read")
	}
	if tool.CallID != "call-1" {
		t.Errorf("Steps[1].CallID = %q, want %q", tool.CallID, "call-1")
	}
	if tool.Status != "completed" {
		t.Errorf("Steps[1].Status = %q, want %q", tool.Status, "completed")
	}
	if tool.DurationMs != 200 {
		t.Errorf("Steps[1].DurationMs = %d, want 200", tool.DurationMs)
	}
}

func TestParseOpencodeJSON_CacheTokens(t *testing.T) {
	raw := joinLines(
		`{"type":"step_start","timestamp":6000000,"part":{}}`,
		`{"type":"step_finish","timestamp":6001000,"part":{"type":"step_finish","reason":"stop","time":{"start":6000000,"end":6001000},"tokens":{"total":100,"input":60,"output":40,"reasoning":0,"cache":{"read":10,"write":5}}}}`,
	)

	got := parseOpencodeJSON(raw)

	if got.Tokens == nil {
		t.Fatalf("Tokens is nil")
	}
	if got.Tokens.CacheRead != 10 {
		t.Errorf("Tokens.CacheRead = %d, want 10", got.Tokens.CacheRead)
	}
	if got.Tokens.CacheWrite != 5 {
		t.Errorf("Tokens.CacheWrite = %d, want 5", got.Tokens.CacheWrite)
	}
}

func joinLines(lines ...string) string {
	return strings.Join(lines, "\n")
}
