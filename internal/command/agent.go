package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
)

// AddAgentCommand handles: add_agent <catalog_id> <name>
type AddAgentCommand struct {
	store *data.Data
}

func (c *AddAgentCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: add_agent <catalog_id> <name>")
	}
	catalogID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid catalog_id: %w", err)
	}
	a, err := c.store.AddAgent(catalogID, params[1])
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"id": a.ID.String(), "name": a.Name})
	return &Response{Message: string(out)}, nil
}

// SetAgentInstructionsCommand handles: set_agent_instructions <agent_id> <instructions...>
type SetAgentInstructionsCommand struct {
	store *data.Data
}

func (c *SetAgentInstructionsCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: set_agent_instructions <agent_id> <instructions>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	instructions := strings.Join(params[1:], " ")
	if err := c.store.SetAgentInstructions(agentID, instructions); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetAgentModelCommand handles: set_agent_model <agent_id> <model>
type SetAgentModelCommand struct {
	store *data.Data
}

func (c *SetAgentModelCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: set_agent_model <agent_id> <model>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.SetAgentModel(agentID, params[1]); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RunAgentCommand handles: run_agent <agent_id> <input...>
type RunAgentCommand struct {
	runner *agent.Runner
}

func (c *RunAgentCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: run_agent <agent_id> <input>")
	}
	agentID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	input := strings.Join(params[1:], " ")
	result, err := c.runner.Run(context.Background(), agentID, input, nil)
	if err != nil {
		return nil, err
	}
	return &Response{Message: result.Response}, nil
}
