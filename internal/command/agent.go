package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/scheduler"
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
// Returns a job_id immediately; poll job_status / job_result for the response.
type RunAgentCommand struct {
	store  *data.Data
	runner *agent.Runner
	sched  *scheduler.Scheduler
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

	// Eagerly validate the agent exists so bad IDs fail before a job is created.
	if _, _, err := c.store.FindAgent(agentID); err != nil {
		return nil, fmt.Errorf("agent not found")
	}

	job, err := c.store.CreateJob("agent_run", ksuid.KSUID{}, ksuid.KSUID{}, input)
	if err != nil {
		return nil, err
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	_ = c.store.SetJobCancel(job.ID, cancel)

	c.sched.Schedule(&scheduler.Activity{
		Job: func() {
			defer cancel()
			_ = c.store.UpdateJob(job.ID, data.JobRunning, "", "")
			result, runErr := c.runner.Run(jobCtx, agentID, input, nil)
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
