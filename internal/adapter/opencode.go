package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type OpencodeAdapter struct {
	Command    string
	WorkingDir string
}

func NewOpencodeAdapter(command, workingDir string) *OpencodeAdapter {
	if command == "" {
		command = "opencode"
	}
	return &OpencodeAdapter{
		Command:    command,
		WorkingDir: workingDir,
	}
}

func (a *OpencodeAdapter) Name() string { return "opencode" }

func (a *OpencodeAdapter) Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error) {
	args := []string{a.Command, "run", "--format", "json", input}
	raw := runCLI(ctx, args, a.WorkingDir, ec)
	parsed := parseOpencodeJSON(raw.Stdout)
	return toExecutionResult(raw, parsed)
}

type opencodeEvent struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"sessionID"`
	Part      struct {
		Type   string `json:"type"`
		Text   string `json:"text"`
		Name   string `json:"name"`
		Tool   string `json:"tool"`
		CallID string `json:"callID"`
		Reason string `json:"reason"`
		Time   *struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"time"`
		State *struct {
			Status   string         `json:"status"`
			Input    any            `json:"input"`
			Output   any            `json:"output"`
			Title    string         `json:"title"`
			Metadata map[string]any `json:"metadata"`
			Time     *struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"time"`
		} `json:"state"`
		Tokens *struct {
			Total     int64 `json:"total"`
			Input     int64 `json:"input"`
			Output    int64 `json:"output"`
			Reasoning int64 `json:"reasoning"`
			Cache     *struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
	} `json:"part"`
}

func parseOpencodeJSON(raw string) parsedOutput {
	var result parsedOutput
	var textParts []string

	var pendingTools []StepDetail
	var stepStartMs int64
	var stepTextParts []string
	var firstTextEventTs int64
	var lastToolOutputs []map[string]any

	scanJSONLines(raw, func(event opencodeEvent) {
		if result.SessionID == "" && event.SessionID != "" {
			result.SessionID = event.SessionID
		}
		switch event.Type {
		case "step_start":
			stepStartMs = event.Timestamp
			stepTextParts = nil
			firstTextEventTs = 0

		case "text":
			textParts = append(textParts, event.Part.Text)
			stepTextParts = append(stepTextParts, event.Part.Text)
			if firstTextEventTs == 0 {
				firstTextEventTs = event.Timestamp
			}

		case "tool_use":
			toolName := event.Part.Tool
			if toolName == "" {
				toolName = event.Part.Name
			}
			step := StepDetail{
				Type:   StepTypeTool,
				Name:   toolName,
				Status: "completed",
				CallID: event.Part.CallID,
			}
			if event.Part.State != nil {
				if event.Part.State.Status != "" {
					step.Status = event.Part.State.Status
				}
				step.Input = event.Part.State.Input
				step.Output = event.Part.State.Output
				step.Title = event.Part.State.Title
				step.Metadata = event.Part.State.Metadata

				step.Display = extractDisplay(event.Part.State.Metadata)

				if event.Part.State.Time != nil {
					step.StartMs = event.Part.State.Time.Start
					step.EndMs = event.Part.State.Time.End
					step.DurationMs = step.EndMs - step.StartMs
				}
			}
			pendingTools = append(pendingTools, step)

		case "step_finish":
			if event.Part.Tokens != nil {
				if result.Tokens == nil {
					result.Tokens = &TokenInfo{}
				}
				result.Tokens.Input += event.Part.Tokens.Input
				result.Tokens.Output += event.Part.Tokens.Output
				result.Tokens.Total += event.Part.Tokens.Total
				result.Tokens.Reasoning += event.Part.Tokens.Reasoning
				if event.Part.Tokens.Cache != nil {
					result.Tokens.CacheRead += event.Part.Tokens.Cache.Read
					result.Tokens.CacheWrite += event.Part.Tokens.Cache.Write
				}
			}

			var toolTimeInStep int64
			var toolsInStep []string
			for _, t := range pendingTools {
				toolTimeInStep += t.DurationMs
				toolsInStep = append(toolsInStep, t.Name)
			}

			genStep := StepDetail{
				Type:        StepTypeGeneration,
				Name:        "llm",
				Status:      "completed",
				Reason:      event.Part.Reason,
				StartMs:     stepStartMs,
				EndMs:       event.Timestamp,
				ToolsCalled: toolsInStep,
			}
			if len(lastToolOutputs) > 0 {
				genStep.Input = lastToolOutputs
			}
			lastToolOutputs = nil
			for _, t := range pendingTools {
				entry := map[string]any{"tool": t.Name}
				if t.Output != nil {
					entry["output"] = t.Output
				}
				lastToolOutputs = append(lastToolOutputs, entry)
			}
			genStep.DurationMs = genStep.EndMs - genStep.StartMs
			genStep.ToolTimeMs = toolTimeInStep
			genStep.LLMInferenceMs = genStep.DurationMs - toolTimeInStep

			if firstTextEventTs > 0 && stepStartMs > 0 {
				ttft := firstTextEventTs - stepStartMs
				genStep.TTFTMs = &ttft
				if result.TTFTMs == nil {
					result.TTFTMs = &ttft
				}
			}

			if event.Part.Tokens != nil {
				tok := &TokenInfo{
					Input:     event.Part.Tokens.Input,
					Output:    event.Part.Tokens.Output,
					Total:     event.Part.Tokens.Total,
					Reasoning: event.Part.Tokens.Reasoning,
				}
				if event.Part.Tokens.Cache != nil {
					tok.CacheRead = event.Part.Tokens.Cache.Read
					tok.CacheWrite = event.Part.Tokens.Cache.Write
				}
				genStep.Tokens = tok
			}
			if event.Part.Reason != "" {
				genStep.Name = fmt.Sprintf("llm (%s)", event.Part.Reason)
			}
			if len(stepTextParts) > 0 {
				genStep.Output = strings.Join(stepTextParts, "")
			} else if len(toolsInStep) > 0 {
				genStep.Output = map[string]any{"toolsCalled": toolsInStep}
			}

			result.Steps = append(result.Steps, genStep)
			result.Steps = append(result.Steps, pendingTools...)
			pendingTools = nil
			stepTextParts = nil
			firstTextEventTs = 0
		}
	})

	result.Text = strings.Join(textParts, "")

	return result
}

func extractDisplay(metadata map[string]any) *ToolDisplay {
	if metadata == nil {
		return nil
	}
	displayRaw, ok := metadata["display"]
	if !ok {
		return nil
	}

	data, err := json.Marshal(displayRaw)
	if err != nil {
		return nil
	}
	var d ToolDisplay
	if err := json.Unmarshal(data, &d); err != nil {
		return nil
	}
	if d.Type == "" && d.Path == "" {
		return nil
	}
	return &d
}
