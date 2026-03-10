package command

import (
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
)

// AddToolCommand handles: add_tool
type AddToolCommand struct {
	store *data.Data
}

func (c *AddToolCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Tool    string `json:"tool"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if p.Tool == "" {
		return nil, fmt.Errorf("tool is required")
	}
	if err := c.store.AddAgentTool(agentID, p.Tool); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RemoveToolCommand handles: remove_tool
type RemoveToolCommand struct {
	store *data.Data
}

func (c *RemoveToolCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Tool    string `json:"tool"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if p.Tool == "" {
		return nil, fmt.Errorf("tool is required")
	}
	if err := c.store.RemoveAgentTool(agentID, p.Tool); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// ListToolsCommand handles: ls_tools
type ListToolsCommand struct {
	store *data.Data
}

func (c *ListToolsCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	a, _, err := c.store.FindAgent(agentID)
	if err != nil {
		return nil, err
	}
	tools := a.Tools
	if tools == nil {
		tools = []string{}
	}
	out, _ := json.Marshal(map[string]interface{}{"tools": tools})
	return &Response{Message: string(out)}, nil
}
