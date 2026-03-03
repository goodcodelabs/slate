package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"slate/internal/llm"
)

// ShellTool executes shell commands and returns stdout/stderr.
type ShellTool struct{}

func NewShellTool() *ShellTool { return &ShellTool{} }

func (t *ShellTool) Definition() llm.ToolDef {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"command":         {"type": "string", "description": "Shell command to execute"},
			"timeout_seconds": {"type": "integer", "description": "Execution timeout in seconds (default 30, max 120)"}
		},
		"required": ["command"]
	}`)
	return llm.ToolDef{
		Name:        "shell",
		Description: "Execute a shell command and return its stdout and stderr output.",
		InputSchema: schema,
	}
}

func (t *ShellTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	timeout := time.Duration(params.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command)
	out, err := cmd.CombinedOutput()

	result := map[string]interface{}{
		"output":   string(out),
		"exit_code": 0,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result["exit_code"] = exitErr.ExitCode()
		} else {
			result["exit_code"] = -1
			result["error"] = err.Error()
		}
	}

	encoded, _ := json.Marshal(result)
	return encoded, nil
}
