package command

import (
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
)

// AddToolCommand handles: add_tool <agent_id> <tool_name>
type AddToolCommand struct {
	store *data.Data
}

func (c *AddToolCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: add_tool <agent_id> <tool_name>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.AddAgentTool(agentID, params[1]); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RemoveToolCommand handles: remove_tool <agent_id> <tool_name>
type RemoveToolCommand struct {
	store *data.Data
}

func (c *RemoveToolCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: remove_tool <agent_id> <tool_name>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.RemoveAgentTool(agentID, params[1]); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// ListToolsCommand handles: ls_tools <agent_id>
type ListToolsCommand struct {
	store *data.Data
}

func (c *ListToolsCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: ls_tools <agent_id>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	agent, _, err := c.store.FindAgent(agentID)
	if err != nil {
		return nil, err
	}
	tools := agent.Tools
	if tools == nil {
		tools = []string{}
	}
	out, _ := json.Marshal(map[string]interface{}{"tools": tools})
	return &Response{Message: string(out)}, nil
}
