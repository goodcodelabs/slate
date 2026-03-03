package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"slate/internal/llm"
)

// FileTool reads or writes files on the local filesystem.
type FileTool struct{}

func NewFileTool() *FileTool { return &FileTool{} }

func (t *FileTool) Definition() llm.ToolDef {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"action":  {"type": "string", "enum": ["read","write","append"], "description": "File operation to perform"},
			"path":    {"type": "string", "description": "Absolute or relative file path"},
			"content": {"type": "string", "description": "Content to write or append (required for write/append)"}
		},
		"required": ["action", "path"]
	}`)
	return llm.ToolDef{
		Name:        "file",
		Description: "Read or write files on the local filesystem.",
		InputSchema: schema,
	}
}

func (t *FileTool) Execute(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Action  string `json:"action"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Basic path sanitisation — reject traversal attempts.
	clean := filepath.Clean(params.Path)
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("path traversal not allowed")
	}

	switch params.Action {
	case "read":
		data, err := os.ReadFile(clean)
		if err != nil {
			return nil, fmt.Errorf("reading file: %w", err)
		}
		out, _ := json.Marshal(map[string]string{"content": string(data)})
		return out, nil

	case "write":
		if err := os.MkdirAll(filepath.Dir(clean), 0755); err != nil {
			return nil, fmt.Errorf("creating directories: %w", err)
		}
		if err := os.WriteFile(clean, []byte(params.Content), 0644); err != nil {
			return nil, fmt.Errorf("writing file: %w", err)
		}
		out, _ := json.Marshal(map[string]string{"status": "ok"})
		return out, nil

	case "append":
		f, err := os.OpenFile(clean, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(params.Content); err != nil {
			return nil, fmt.Errorf("appending to file: %w", err)
		}
		out, _ := json.Marshal(map[string]string{"status": "ok"})
		return out, nil

	default:
		return nil, fmt.Errorf("unknown action %q (must be read, write, or append)", params.Action)
	}
}
