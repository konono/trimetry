package adapter

import (
	"context"
	"strings"
)

type CodexAdapter struct {
	Command    string
	WorkingDir string
	Sandbox    string
	Hermetic   bool
}

func NewCodexAdapter(command, workingDir string, options map[string]string) *CodexAdapter {
	if command == "" {
		command = "codex"
	}
	a := &CodexAdapter{
		Command:    command,
		WorkingDir: workingDir,
		Sandbox:    "danger-full-access",
	}
	if v, ok := options["sandbox"]; ok {
		a.Sandbox = v
	}
	if options["hermetic"] == "true" {
		a.Hermetic = true
	}
	return a
}

func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) Execute(ctx context.Context, input string, ec ExecutionContext) (*ExecutionResult, error) {
	args := []string{a.Command, "exec", "--json", "--sandbox", a.Sandbox}
	if a.Hermetic {
		args = append(args, "--ignore-user-config", "--ignore-rules")
	}
	args = append(args, input)

	raw := runCLI(ctx, args, a.WorkingDir, ec)
	parsed := parseCodexJSON(raw.Stdout)
	return toExecutionResult(raw, parsed)
}

type codexEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id,omitempty"`
	Item     *struct {
		ID      string `json:"id,omitempty"`
		Type    string `json:"type,omitempty"`
		Text    string `json:"text,omitempty"`
		Content string `json:"content,omitempty"`
		Name    string `json:"name,omitempty"`
		Status  string `json:"status,omitempty"`
		Command *struct {
			Cmd string `json:"cmd,omitempty"`
		} `json:"command,omitempty"`
	} `json:"item,omitempty"`
	Usage *struct {
		InputTokens    int64 `json:"input_tokens"`
		OutputTokens   int64 `json:"output_tokens"`
		CachedTokens   int64 `json:"cached_input_tokens"`
		ReasoningTokens int64 `json:"reasoning_output_tokens"`
	} `json:"usage,omitempty"`
}

func parseCodexJSON(raw string) parsedOutput {
	var result parsedOutput
	var textParts []string

	scanJSONLines(raw, func(event codexEvent) {
		switch event.Type {
		case "turn.completed":
			if event.Usage != nil {
				if result.Tokens == nil {
					result.Tokens = &TokenInfo{}
				}
				result.Tokens.Input += event.Usage.InputTokens
				result.Tokens.Output += event.Usage.OutputTokens
				result.Tokens.Total += event.Usage.InputTokens + event.Usage.OutputTokens
				result.Tokens.Reasoning += event.Usage.ReasoningTokens
				result.Tokens.CacheRead += event.Usage.CachedTokens
			}

		case "item.completed":
			if event.Item == nil {
				return
			}
			switch event.Item.Type {
			case "agent_message":
				text := event.Item.Text
				if text == "" {
					text = event.Item.Content
				}
				if text != "" {
					textParts = append(textParts, text)
				}
			}
		}
	})

	result.Text = strings.Join(textParts, "")

	return result
}
