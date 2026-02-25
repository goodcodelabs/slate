package tools

import (
	"context"
	"encoding/json"

	"slate/internal/llm"
)

// Tool is anything the orchestrator can execute on an agent's behalf.
type Tool interface {
	// Definition returns the metadata the LLM needs to call this tool.
	Definition() llm.ToolDef

	// Execute runs the tool with the given JSON input and returns JSON output.
	Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
