package adapter

import (
	"strings"
	"testing"
)

func TestParseClaudeJSON(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantTextParts []string // substrings that must appear in output
		wantInputTok  int64
		wantOutputTok int64
		wantCostUSD   *float64
		wantTTFTMs    *int64
		wantStepTypes []string
	}{
		{
			name: "full conversation with result",
			raw: strings.Join([]string{
				`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"Hello"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
				`{"type":"assistant","timestamp":"2024-01-01T00:00:02.000Z","message":{"model":"claude-3","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{"path":"/tmp"}}],"usage":{"input_tokens":80,"output_tokens":30,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
				`{"type":"user","timestamp":"2024-01-01T00:00:03.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu-1","content":"result text"}]}}`,
				`{"type":"result","result":"Final answer","total_cost_usd":0.005,"num_turns":2,"duration_ms":3000,"duration_api_ms":2500,"ttft_ms":150,"usage":{"input_tokens":180,"output_tokens":80,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
			}, "\n"),
			wantTextParts: []string{"Hello"},
			wantInputTok:  180,
			wantOutputTok: 80,
			wantCostUSD:   float64Ptr(0.005),
			wantTTFTMs:    int64Ptr(150),
			wantStepTypes: []string{
				StepTypeGeneration, // first assistant (text)
				StepTypeGeneration, // second assistant (tool_use)
				StepTypeTool,       // tool step from the second assistant
			},
		},
		{
			name:          "empty input",
			raw:           "",
			wantTextParts: nil,
		},
		{
			name: "result only with no assistant events",
			raw:  `{"type":"result","result":"standalone result","total_cost_usd":0.001,"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
			wantTextParts: []string{"standalone result"},
			wantInputTok:  10,
			wantOutputTok: 5,
			wantCostUSD:   float64Ptr(0.001),
		},
		{
			name: "single assistant text only",
			raw:  `{"type":"assistant","timestamp":"2024-06-01T12:00:00.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"Just text"}],"usage":{"input_tokens":20,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
			wantTextParts: []string{"Just text"},
			wantStepTypes: []string{StepTypeGeneration},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClaudeJSON(tt.raw)

			for _, part := range tt.wantTextParts {
				if !strings.Contains(got.Text, part) {
					t.Errorf("Text %q does not contain %q", got.Text, part)
				}
			}

			if tt.wantInputTok > 0 || tt.wantOutputTok > 0 {
				if got.Tokens == nil {
					t.Fatalf("Tokens is nil, want non-nil")
				}
				if got.Tokens.Input != tt.wantInputTok {
					t.Errorf("Tokens.Input = %d, want %d", got.Tokens.Input, tt.wantInputTok)
				}
				if got.Tokens.Output != tt.wantOutputTok {
					t.Errorf("Tokens.Output = %d, want %d", got.Tokens.Output, tt.wantOutputTok)
				}
			}

			if tt.wantCostUSD != nil {
				if got.CostUSD == nil {
					t.Fatalf("CostUSD is nil, want %f", *tt.wantCostUSD)
				}
				if *got.CostUSD != *tt.wantCostUSD {
					t.Errorf("CostUSD = %f, want %f", *got.CostUSD, *tt.wantCostUSD)
				}
			}

			if tt.wantTTFTMs != nil {
				if got.TTFTMs == nil {
					t.Fatalf("TTFTMs is nil, want %d", *tt.wantTTFTMs)
				}
				if *got.TTFTMs != *tt.wantTTFTMs {
					t.Errorf("TTFTMs = %d, want %d", *got.TTFTMs, *tt.wantTTFTMs)
				}
			}

			if tt.wantStepTypes != nil {
				if len(got.Steps) != len(tt.wantStepTypes) {
					t.Fatalf("Steps count = %d, want %d", len(got.Steps), len(tt.wantStepTypes))
				}
				for i, wantType := range tt.wantStepTypes {
					if got.Steps[i].Type != wantType {
						t.Errorf("Steps[%d].Type = %q, want %q", i, got.Steps[i].Type, wantType)
					}
				}
			}
		})
	}
}

func TestParseClaudeJSON_ToolDetails(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"tool_use","id":"tu-42","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":50,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:02.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu-42","content":"file1.go\nfile2.go"}]}}`,
		`{"type":"result","result":"done","usage":{"input_tokens":50,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) < 2 {
		t.Fatalf("Steps count = %d, want at least 2", len(got.Steps))
	}

	gen := got.Steps[0]
	if gen.Type != StepTypeGeneration {
		t.Errorf("Steps[0].Type = %q, want %q", gen.Type, StepTypeGeneration)
	}
	if gen.Name != "llm (tool_use)" {
		t.Errorf("Steps[0].Name = %q, want %q", gen.Name, "llm (tool_use)")
	}
	if len(gen.ToolsCalled) != 1 || gen.ToolsCalled[0] != "Bash" {
		t.Errorf("Steps[0].ToolsCalled = %v, want [Bash]", gen.ToolsCalled)
	}

	tool := got.Steps[1]
	if tool.Type != StepTypeTool {
		t.Errorf("Steps[1].Type = %q, want %q", tool.Type, StepTypeTool)
	}
	if tool.Name != "Bash" {
		t.Errorf("Steps[1].Name = %q, want %q", tool.Name, "Bash")
	}
	if tool.CallID != "tu-42" {
		t.Errorf("Steps[1].CallID = %q, want %q", tool.CallID, "tu-42")
	}
}

func TestParseClaudeJSON_ToolResultOutput(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"tool_use","id":"tu-99","name":"Read","input":{"path":"/tmp/x"}}],"usage":{"input_tokens":30,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:02.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu-99","content":"hello from tool"}]}}`,
		`{"type":"result","result":"ok","usage":{"input_tokens":30,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) < 2 {
		t.Fatalf("Steps count = %d, want at least 2", len(got.Steps))
	}
	tool := got.Steps[1]
	outStr, ok := tool.Output.(string)
	if !ok {
		t.Fatalf("Steps[1].Output type = %T, want string", tool.Output)
	}
	if outStr != "hello from tool" {
		t.Errorf("Steps[1].Output = %q, want %q", outStr, "hello from tool")
	}
}

func TestParseClaudeJSON_ErrorTool(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"tool_use","id":"tu-err","name":"Bash","input":{"command":"false"}}],"usage":{"input_tokens":30,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:02.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu-err","content":"exit 1","is_error":true}]}}`,
		`{"type":"result","result":"failed","usage":{"input_tokens":30,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) < 2 {
		t.Fatalf("Steps count = %d, want at least 2", len(got.Steps))
	}
	tool := got.Steps[1]
	if tool.Status != "error" {
		t.Errorf("Steps[1].Status = %q, want %q", tool.Status, "error")
	}
}

func TestParseClaudeJSON_SessionID(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"ses-abc-123","cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"result","result":"hi","session_id":"ses-abc-123","total_cost_usd":0.001,"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if got.SessionID != "ses-abc-123" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "ses-abc-123")
	}
}

func TestParseClaudeJSON_SessionID_FromResult(t *testing.T) {
	raw := `{"type":"result","result":"hi","session_id":"ses-from-result","total_cost_usd":0.001,"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`

	got := parseClaudeJSON(raw)

	if got.SessionID != "ses-from-result" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "ses-from-result")
	}
}

func TestParseClaudeJSON_Thinking(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"thinking","thinking":"Let me think about this..."},{"type":"text","text":"The answer is 4."}],"usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"result","result":"The answer is 4.","usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens_details":{"thinking_tokens":15}}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) < 1 {
		t.Fatalf("Steps count = %d, want at least 1", len(got.Steps))
	}
	gen := got.Steps[0]
	if len(gen.ThinkingParts) != 1 {
		t.Fatalf("ThinkingParts count = %d, want 1", len(gen.ThinkingParts))
	}
	if gen.ThinkingParts[0] != "Let me think about this..." {
		t.Errorf("ThinkingParts[0] = %q, want %q", gen.ThinkingParts[0], "Let me think about this...")
	}
	if !strings.Contains(got.Text, "The answer is 4.") {
		t.Errorf("Text %q does not contain expected text", got.Text)
	}
}

func TestParseClaudeJSON_ThinkingTokens(t *testing.T) {
	raw := `{"type":"result","result":"answer","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens_details":{"thinking_tokens":200}}}`

	got := parseClaudeJSON(raw)

	if got.Tokens == nil {
		t.Fatal("Tokens is nil")
	}
	if got.Tokens.Reasoning != 200 {
		t.Errorf("Tokens.Reasoning = %d, want 200", got.Tokens.Reasoning)
	}
}

func TestParseClaudeJSON_ThinkingTokensZeroWhenAbsent(t *testing.T) {
	raw := `{"type":"result","result":"answer","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`

	got := parseClaudeJSON(raw)

	if got.Tokens == nil {
		t.Fatal("Tokens is nil")
	}
	if got.Tokens.Reasoning != 0 {
		t.Errorf("Tokens.Reasoning = %d, want 0", got.Tokens.Reasoning)
	}
}

func TestParseTimestampMs(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want int64
	}{
		{
			name: "RFC3339Nano",
			ts:   "2024-01-01T00:00:01.000Z",
			want: 1704067201000,
		},
		{
			name: "empty string",
			ts:   "",
			want: 0,
		},
		{
			name: "invalid format",
			ts:   "not-a-timestamp",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestampMs(tt.ts)
			if got != tt.want {
				t.Errorf("parseTimestampMs(%q) = %d, want %d", tt.ts, got, tt.want)
			}
		})
	}
}

func TestParseClaudeJSON_SplitThinkingMerge(t *testing.T) {
	// Claude Code splits one API turn into two assistant events:
	// 1. thinking-only  2. text/tool_use
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"thinking","thinking":"reasoning here"}],"usage":{"input_tokens":100,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:02.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"answer"}],"usage":{"input_tokens":0,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"result","result":"answer","usage":{"input_tokens":100,"output_tokens":25,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) != 1 {
		t.Fatalf("Steps count = %d, want 1 (merged generation)", len(got.Steps))
	}
	gen := got.Steps[0]
	if len(gen.ThinkingParts) != 1 || gen.ThinkingParts[0] != "reasoning here" {
		t.Errorf("ThinkingParts = %v, want [reasoning here]", gen.ThinkingParts)
	}
	if gen.Tokens == nil {
		t.Fatal("Tokens is nil")
	}
	if gen.Tokens.Input != 100 {
		t.Errorf("Tokens.Input = %d, want 100", gen.Tokens.Input)
	}
	if gen.Tokens.Output != 25 {
		t.Errorf("Tokens.Output = %d, want 25", gen.Tokens.Output)
	}
}

func TestParseClaudeJSON_ThinkingAfterToolUse(t *testing.T) {
	// turn1: tool_use → tool_result → turn2: thinking-only → text
	// The thinking-only event must NOT merge into turn1's generation.
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"tool_use","id":"tu-1","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":50,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:02.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu-1","content":"file.go"}]}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:03.000Z","message":{"model":"claude-3","content":[{"type":"thinking","thinking":"now I know"}],"usage":{"input_tokens":80,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:04.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":0,"output_tokens":15,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"result","result":"done","usage":{"input_tokens":130,"output_tokens":30,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	// Expect: gen1 (tool_use, no thinking), tool1, gen2 (thinking + text)
	if len(got.Steps) != 3 {
		t.Fatalf("Steps count = %d, want 3", len(got.Steps))
	}
	gen1 := got.Steps[0]
	if len(gen1.ThinkingParts) != 0 {
		t.Errorf("gen1.ThinkingParts = %v, want empty", gen1.ThinkingParts)
	}
	if got.Steps[1].Type != StepTypeTool {
		t.Errorf("Steps[1].Type = %q, want %q", got.Steps[1].Type, StepTypeTool)
	}
	gen2 := got.Steps[2]
	if len(gen2.ThinkingParts) != 1 || gen2.ThinkingParts[0] != "now I know" {
		t.Errorf("gen2.ThinkingParts = %v, want [now I know]", gen2.ThinkingParts)
	}
	if !strings.Contains(got.Text, "done") {
		t.Errorf("Text %q missing 'done'", got.Text)
	}
}

func TestParseClaudeJSON_MultiTurnThinking(t *testing.T) {
	// turn1: thinking→text, turn2: thinking→text
	// Each turn's thinking must stay with its own generation.
	raw := strings.Join([]string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:01.000Z","message":{"model":"claude-3","content":[{"type":"thinking","thinking":"t1"}],"usage":{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:02.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"first"}],"usage":{"input_tokens":0,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:03.000Z","message":{"content":[{"type":"text","text":"next"}]}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:04.000Z","message":{"model":"claude-3","content":[{"type":"thinking","thinking":"t2"}],"usage":{"input_tokens":20,"output_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2024-01-01T00:00:05.000Z","message":{"model":"claude-3","content":[{"type":"text","text":"second"}],"usage":{"input_tokens":0,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		`{"type":"result","result":"second","usage":{"input_tokens":30,"output_tokens":26,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
	}, "\n")

	got := parseClaudeJSON(raw)

	if len(got.Steps) != 2 {
		t.Fatalf("Steps count = %d, want 2", len(got.Steps))
	}
	if got.Steps[0].ThinkingParts[0] != "t1" {
		t.Errorf("Steps[0].ThinkingParts[0] = %q, want t1", got.Steps[0].ThinkingParts[0])
	}
	if got.Steps[1].ThinkingParts[0] != "t2" {
		t.Errorf("Steps[1].ThinkingParts[0] = %q, want t2", got.Steps[1].ThinkingParts[0])
	}
}

func float64Ptr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64       { return &v }
