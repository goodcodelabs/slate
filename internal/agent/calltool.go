package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/llm"
	"slate/internal/tools"
)

// callAgentTool implements tools.Tool by delegating to the Runner.
// Defined in this package to avoid an import cycle between agent and tools/builtin.
type callAgentTool struct {
	runner *Runner
}

func (t *callAgentTool) Definition() llm.ToolDef {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_id": {"type": "string", "description": "KSUID of the agent to call"},
			"input":    {"type": "string", "description": "Input prompt to pass to the agent"}
		},
		"required": ["agent_id", "input"]
	}`)
	return llm.ToolDef{
		Name:        "call_agent",
		Description: "Delegate a subtask to another agent and return its response. Use this to hand off specialised work.",
		InputSchema: schema,
	}
}

func (t *callAgentTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		AgentID string `json:"agent_id"`
		Input   string `json:"input"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	agentID, err := ksuid.Parse(params.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	result, err := t.runner.Run(ctx, agentID, params.Input, nil)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"response": result.Response})
	return out, nil
}

// RegisterCallAgentTool adds the call_agent tool to reg using this runner.
// Call this after both the runner and registry have been created.
func (r *Runner) RegisterCallAgentTool(reg *tools.Registry) {
	reg.Register(&callAgentTool{runner: r})
}
