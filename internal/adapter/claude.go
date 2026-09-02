package adapter

import (
	"context"
	"strings"
	"time"
)

type ClaudeAdapter struct {
	Command        string
	WorkingDir     string
	PermissionMode string
	Model          string
	Effort         string
	MaxBudgetUSD   string
	Bare           bool
	Continue       bool
}

func NewClaudeAdapter(command, workingDir string, options map[string]string) *ClaudeAdapter {
	if command == "" {
		command = "claude"
	}
	a := &ClaudeAdapter{
		Command:        command,
		WorkingDir:     workingDir,
		PermissionMode: "bypassPermissions",
	}
	if v, ok := options["permission_mode"]; ok {
		a.PermissionMode = v
	}
	if v, ok := options["model"]; ok {
		a.Model = v
	}
	if v, ok := options["effort"]; ok {
		a.Effort = v
	}
	if v, ok := options["max_budget_usd"]; ok {
		a.MaxBudgetUSD = v
	}
	if options["bare"] == "true" {
		a.Bare = true
	}
	if options["continue_session"] == "true" {
		a.Continue = true
	}
	return a
}

func (a *ClaudeAdapter) Name() string { return "claude" }

func (a *ClaudeAdapter) Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error) {
	args := []string{a.Command, "-p", "--output-format", "stream-json", "--verbose", "--permission-mode", a.PermissionMode}
	if a.Bare {
		args = append(args, "--bare")
	}
	if a.Continue {
		args = append(args, "--continue")
	}
	model := a.Model
	if model == "" {
		model = ec.ModelName
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if a.Effort != "" {
		args = append(args, "--effort", a.Effort)
	}
	if a.MaxBudgetUSD != "" {
		args = append(args, "--max-budget-usd", a.MaxBudgetUSD)
	}
	args = append(args, input)

	raw := runCLI(ctx, args, a.WorkingDir, ec)
	parsed := parseClaudeJSON(raw.Stdout)
	backfillStepTimestamps(parsed.Steps, raw.StartedAt, raw.FinishedAt)
	return toExecutionResult(raw, parsed)
}

type claudeEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`

	Message *struct {
		Model   string `json:"model,omitempty"`
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text,omitempty"`
			Thinking  string `json:"thinking,omitempty"`
			Name      string `json:"name,omitempty"`
			ID        string `json:"id,omitempty"`
			ToolUseID string `json:"tool_use_id,omitempty"`
			Content   string `json:"content,omitempty"`
			IsError   bool   `json:"is_error,omitempty"`
			Input     any    `json:"input,omitempty"`
		} `json:"content"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message,omitempty"`

	ToolUseResult *struct {
		Stdout      string `json:"stdout,omitempty"`
		Stderr      string `json:"stderr,omitempty"`
		Interrupted bool   `json:"interrupted,omitempty"`
	} `json:"tool_use_result,omitempty"`

	Result        string   `json:"result,omitempty"`
	TotalCostUSD  *float64 `json:"total_cost_usd,omitempty"`
	NumTurns      *int     `json:"num_turns,omitempty"`
	DurationMs    *int64   `json:"duration_ms,omitempty"`
	DurationAPIMs *int64   `json:"duration_api_ms,omitempty"`
	TTFTMs        *int64   `json:"ttft_ms,omitempty"`

	Usage *struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokensDetails      *struct {
			ThinkingTokens int64 `json:"thinking_tokens"`
		} `json:"output_tokens_details,omitempty"`
	} `json:"usage,omitempty"`
}

type pendingGeneration struct {
	step           StepDetail
	textParts      []string
	thinkingParts  []string
	toolsCalled    []string
	toolTime       int64
	tools          []StepDetail
}

func parseClaudeJSON(raw string) parsedOutput {
	var result parsedOutput
	var textParts []string

	var pendingGen *pendingGeneration
	var lastUserInput any
	pendingToolMap := make(map[string]int) // callID -> index in pendingGen.tools

	scanJSONLines(raw, func(event claudeEvent) {
		eventMs := parseTimestampMs(event.Timestamp)

		switch event.Type {
		case "system":
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
			return

		case "assistant":
			// Claude Code splits one API turn into a thinking-only assistant
			// event followed by a text/tool_use assistant event.  When the
			// pending generation contains only thinking (no text or tools),
			// we merge the next assistant event into it rather than finalizing
			// an empty generation.
			if pendingGen != nil {
				pendingIsThinkingOnly := len(pendingGen.thinkingParts) > 0 &&
					len(pendingGen.textParts) == 0 && len(pendingGen.toolsCalled) == 0
				if !pendingIsThinkingOnly {
					finalizeGeneration(pendingGen, eventMs, &result)
					pendingGen = nil
					pendingToolMap = make(map[string]int)
				}
			}

			if pendingGen == nil {
				pendingGen = &pendingGeneration{
					step: StepDetail{
						Type:    StepTypeGeneration,
						Name:    "llm",
						Status:  "completed",
						StartMs: eventMs,
						Input:   lastUserInput,
					},
				}
			}

			if event.Message != nil {
				if event.Message.Model != "" {
					pendingGen.step.Model = event.Message.Model
				}

				for _, c := range event.Message.Content {
					switch c.Type {
					case "thinking":
						if c.Thinking != "" {
							pendingGen.thinkingParts = append(pendingGen.thinkingParts, c.Thinking)
						}
					case "text":
						textParts = append(textParts, c.Text)
						pendingGen.textParts = append(pendingGen.textParts, c.Text)
					case "tool_use":
						if c.Name != "" {
							pendingGen.toolsCalled = append(pendingGen.toolsCalled, c.Name)

							toolStep := StepDetail{
								Type:    StepTypeTool,
								Name:    c.Name,
								CallID:  c.ID,
								Status:  "completed",
								StartMs: eventMs,
								Input:   c.Input,
							}
							pendingToolMap[c.ID] = len(pendingGen.tools)
							pendingGen.tools = append(pendingGen.tools, toolStep)
						}
					}
				}

				if event.Message.Usage != nil {
					if pendingGen.step.Tokens == nil {
						pendingGen.step.Tokens = &TokenInfo{}
					}
					pendingGen.step.Tokens.Input += event.Message.Usage.InputTokens
					pendingGen.step.Tokens.Output += event.Message.Usage.OutputTokens
					pendingGen.step.Tokens.Total = pendingGen.step.Tokens.Input + pendingGen.step.Tokens.Output
					pendingGen.step.Tokens.CacheRead += event.Message.Usage.CacheReadInputTokens
					pendingGen.step.Tokens.CacheWrite += event.Message.Usage.CacheCreationInputTokens
				}
			}

		case "user":
			if event.Message != nil {
				lastUserInput = event.Message.Content
			}
			if event.Message == nil || pendingGen == nil {
				return
			}
			for _, c := range event.Message.Content {
				if c.Type != "tool_result" {
					continue
				}
				idx, ok := pendingToolMap[c.ToolUseID]
				if !ok {
					continue
				}

				pendingGen.tools[idx].EndMs = eventMs
				pendingGen.tools[idx].DurationMs = eventMs - pendingGen.tools[idx].StartMs

				if event.ToolUseResult != nil {
					pendingGen.tools[idx].Output = event.ToolUseResult.Stdout
				} else if c.Content != "" {
					pendingGen.tools[idx].Output = c.Content
				}

				if c.IsError {
					pendingGen.tools[idx].Status = "error"
				}

				pendingGen.toolTime += pendingGen.tools[idx].DurationMs
			}

		case "result":
			if pendingGen != nil {
				finalizeGeneration(pendingGen, eventMs, &result)
				pendingGen = nil
			}

			if event.Result != "" && len(textParts) == 0 {
				textParts = append(textParts, event.Result)
			}
			if event.TotalCostUSD != nil {
				result.CostUSD = event.TotalCostUSD
			}
			if event.Usage != nil {
				var reasoning int64
				if event.Usage.OutputTokensDetails != nil {
					reasoning = event.Usage.OutputTokensDetails.ThinkingTokens
				}
				result.Tokens = &TokenInfo{
					Input:      event.Usage.InputTokens,
					Output:     event.Usage.OutputTokens,
					Total:      event.Usage.InputTokens + event.Usage.OutputTokens,
					Reasoning:  reasoning,
					CacheRead:  event.Usage.CacheReadInputTokens,
					CacheWrite: event.Usage.CacheCreationInputTokens,
				}
			}
			if event.TTFTMs != nil {
				result.TTFTMs = event.TTFTMs
			}
			if event.SessionID != "" {
				result.SessionID = event.SessionID
			}
		}
	})

	if pendingGen != nil {
		finalizeGeneration(pendingGen, 0, &result)
	}

	result.Text = strings.Join(textParts, "")

	return result
}

func finalizeGeneration(gen *pendingGeneration, endMs int64, result *parsedOutput) {
	if endMs > 0 {
		gen.step.EndMs = endMs
		gen.step.DurationMs = gen.step.EndMs - gen.step.StartMs
	}
	gen.step.ToolTimeMs = gen.toolTime
	gen.step.LLMInferenceMs = gen.step.DurationMs - gen.toolTime
	if gen.step.LLMInferenceMs < 0 {
		gen.step.LLMInferenceMs = 0
	}
	gen.step.ToolsCalled = gen.toolsCalled
	gen.step.ThinkingParts = gen.thinkingParts

	if len(gen.textParts) > 0 {
		gen.step.Output = strings.Join(gen.textParts, "")
	}
	if len(gen.toolsCalled) > 0 {
		if gen.step.Name == "llm" {
			gen.step.Name = "llm (tool_use)"
		}
		if gen.step.Output == nil {
			gen.step.Output = map[string]any{"toolsCalled": gen.toolsCalled}
		}
	}

	result.Steps = append(result.Steps, gen.step)
	result.Steps = append(result.Steps, gen.tools...)
}

func backfillStepTimestamps(steps []StepDetail, startedAt, finishedAt time.Time) {
	startMs := startedAt.UnixMilli()
	endMs := finishedAt.UnixMilli()
	for i := range steps {
		changed := false
		if steps[i].StartMs <= 0 {
			steps[i].StartMs = startMs
			changed = true
		}
		if steps[i].EndMs <= 0 {
			steps[i].EndMs = endMs
			changed = true
		}
		if changed && steps[i].DurationMs <= 0 {
			steps[i].DurationMs = steps[i].EndMs - steps[i].StartMs
			if steps[i].Type == StepTypeGeneration {
				steps[i].LLMInferenceMs = steps[i].DurationMs - steps[i].ToolTimeMs
				if steps[i].LLMInferenceMs < 0 {
					steps[i].LLMInferenceMs = 0
				}
			}
		}
	}
}

func parseTimestampMs(ts string) int64 {
	if ts == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", ts)
		if err != nil {
			return 0
		}
	}
	return t.UnixMilli()
}
