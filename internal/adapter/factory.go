package adapter

import (
	"fmt"
	"strings"
)

var supportedTypes = []string{"opencode", "claude", "codex", "cursor", "fake"}

func SupportedTypes() []string {
	return supportedTypes
}

func NewAdapter(adapterType string, options map[string]string) (ApplicationAdapter, error) {
	command := options["command"]
	workingDir := options["working_directory"]

	switch adapterType {
	case "opencode":
		return NewOpencodeAdapter(command, workingDir), nil
	case "claude":
		return NewClaudeAdapter(command, workingDir, options), nil
	case "codex":
		return NewCodexAdapter(command, workingDir, options), nil
	case "cursor":
		return NewCursorAdapter(command, workingDir, options), nil
	case "fake":
		return NewFakeAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %q (supported: %s)",
			adapterType, strings.Join(supportedTypes, ", "))
	}
}
