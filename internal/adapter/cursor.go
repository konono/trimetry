package adapter

import (
	"context"
	"strings"
)

type CursorAdapter struct {
	Command     string
	WorkingDir  string
	Force       bool
	ApproveMCPs bool
	Continue    bool
}

func NewCursorAdapter(command, workingDir string, options map[string]string) *CursorAdapter {
	if command == "" {
		command = "agent"
	}
	a := &CursorAdapter{
		Command:     command,
		WorkingDir:  workingDir,
		Force:       true,
		ApproveMCPs: true,
	}
	if options["force"] == "false" {
		a.Force = false
	}
	if options["approve_mcps"] == "false" {
		a.ApproveMCPs = false
	}
	if options["continue_session"] == "true" {
		a.Continue = true
	}
	return a
}

func (a *CursorAdapter) Name() string { return "cursor" }

func (a *CursorAdapter) Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error) {
	args := []string{a.Command, "-p", "--output-format", "stream-json"}
	if a.Force {
		args = append(args, "--force")
	}
	if a.ApproveMCPs {
		args = append(args, "--approve-mcps")
	}
	if a.Continue {
		args = append(args, "--continue")
	}
	args = append(args, input)

	raw := runCLI(ctx, args, a.WorkingDir, ec)
	parsed := parseCursorJSON(raw.Stdout)
	return toExecutionResult(raw, parsed)
}

type cursorEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`

	SessionID string `json:"session_id,omitempty"`

	Message *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
	} `json:"message,omitempty"`

	ToolCall *struct {
		CallID string `json:"call_id,omitempty"`
		Name   string `json:"name,omitempty"`
	} `json:"tool_call,omitempty"`

	Result      string `json:"result,omitempty"`
	DurationMs  *int64 `json:"duration_ms,omitempty"`
	DurationAPIMs *int64 `json:"duration_api_ms,omitempty"`
	IsError     bool   `json:"is_error,omitempty"`
}

func parseCursorJSON(raw string) parsedOutput {
	var result parsedOutput
	var textParts []string

	scanJSONLines(raw, func(event cursorEvent) {
		switch event.Type {
		case "assistant":
			if event.Message != nil {
				for _, c := range event.Message.Content {
					if c.Type == "text" && c.Text != "" {
						textParts = append(textParts, c.Text)
					}
				}
			}

		case "result":
			if event.Result != "" && len(textParts) == 0 {
				textParts = append(textParts, event.Result)
			}
		}
	})

	result.Text = strings.Join(textParts, "")

	return result
}
