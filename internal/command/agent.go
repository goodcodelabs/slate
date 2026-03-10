package command

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/scheduler"
)

// AddAgentCommand handles: add_agent
type AddAgentCommand struct {
	store *data.Data
}

func (c *AddAgentCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		CatalogID string `json:"catalog_id"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	catalogID, err := ksuid.Parse(p.CatalogID)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog_id: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	a, err := c.store.AddAgent(catalogID, p.Name)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"id": a.ID.String(), "name": a.Name})
	return &Response{Message: string(out)}, nil
}

// DelAgentCommand handles: del_agent
type DelAgentCommand struct {
	store *data.Data
}

func (c *DelAgentCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
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
	if err := c.store.RemoveAgent(agentID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetAgentInstructionsCommand handles: set_agent_instructions
type SetAgentInstructionsCommand struct {
	store *data.Data
}

func (c *SetAgentInstructionsCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID      string `json:"agent_id"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.SetAgentInstructions(agentID, p.Instructions); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetAgentModelCommand handles: set_agent_model
type SetAgentModelCommand struct {
	store *data.Data
}

func (c *SetAgentModelCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if p.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := c.store.SetAgentModel(agentID, p.Model); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RunAgentCommand handles: run_agent
// Returns a job_id immediately; poll job_status / job_result for the response.
type RunAgentCommand struct {
	store  *data.Data
	runner *agent.Runner
	sched  *scheduler.Scheduler
}

func (c *RunAgentCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Input   string `json:"input"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if p.Input == "" {
		return nil, fmt.Errorf("input is required")
	}

	// Eagerly validate the agent exists so bad IDs fail before a job is created.
	if _, _, err := c.store.FindAgent(agentID); err != nil {
		return nil, fmt.Errorf("agent not found")
	}

	job, err := c.store.CreateJob("agent_run", ksuid.KSUID{}, ksuid.KSUID{}, p.Input)
	if err != nil {
		return nil, err
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	_ = c.store.SetJobCancel(job.ID, cancel)

	c.sched.Schedule(&scheduler.Activity{
		Job: func() {
			defer cancel()
			_ = c.store.UpdateJob(job.ID, data.JobRunning, "", "")
			result, runErr := c.runner.Run(jobCtx, agentID, p.Input, nil)
			if runErr != nil {
				_ = c.store.UpdateJob(job.ID, data.JobFailed, "", runErr.Error())
			} else {
				_ = c.store.UpdateJob(job.ID, data.JobCompleted, result.Response, "")
			}
		},
	})

	out, _ := json.Marshal(map[string]string{"job_id": job.ID.String()})
	return &Response{Message: string(out)}, nil
}
