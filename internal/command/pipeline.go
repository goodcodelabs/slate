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

// ListPipelinesCommand handles: ls_pipelines <workspace_id>
type ListPipelinesCommand struct {
	store *data.Data
}

func (c *ListPipelinesCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: ls_pipelines <workspace_id>")
	}
	wsID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	pipelines, err := c.store.ListPipelines(wsID)
	if err != nil {
		return nil, err
	}

	type stepSummary struct {
		AgentID string `json:"agent_id"`
		Mode    string `json:"mode"`
	}
	type pipelineSummary struct {
		ID   string        `json:"id"`
		Name string        `json:"name"`
		Steps []stepSummary `json:"steps"`
	}

	summaries := make([]pipelineSummary, 0, len(pipelines))
	for _, p := range pipelines {
		steps := make([]stepSummary, 0, len(p.Steps))
		for _, s := range p.Steps {
			steps = append(steps, stepSummary{AgentID: s.AgentID.String(), Mode: string(s.Mode)})
		}
		summaries = append(summaries, pipelineSummary{ID: p.ID.String(), Name: p.Name, Steps: steps})
	}

	out, _ := json.Marshal(map[string]interface{}{"pipelines": summaries})
	return &Response{Message: string(out)}, nil
}

// CreatePipelineCommand handles: create_pipeline <workspace_id> <name>
type CreatePipelineCommand struct {
	store *data.Data
}

func (c *CreatePipelineCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: create_pipeline <workspace_id> <name>")
	}
	wsID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	name := strings.Join(params[1:], " ")
	p, err := c.store.NewPipeline(wsID, name)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"pipeline_id": p.ID.String()})
	return &Response{Message: string(out)}, nil
}

// AddPipelineStepCommand handles: add_pipeline_step <pipeline_id> <agent_id> <mode>
type AddPipelineStepCommand struct {
	store *data.Data
}

func (c *AddPipelineStepCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 3 {
		return nil, fmt.Errorf("usage: add_pipeline_step <pipeline_id> <agent_id> <mode>")
	}
	pipelineID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline_id: %w", err)
	}
	agentID, err := ksuid.Parse(params[1])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	mode := data.StepMode(params[2])
	if mode != data.StepModeSequential && mode != data.StepModeParallel {
		return nil, fmt.Errorf("invalid mode: must be 'sequential' or 'parallel'")
	}
	if err := c.store.AddPipelineStep(pipelineID, agentID, mode); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RunPipelineCommand handles: run_pipeline <pipeline_id> <input>
// Creates an async job, schedules pipeline execution, and returns the job ID immediately.
type RunPipelineCommand struct {
	store  *data.Data
	runner *agent.Runner
	sched  *scheduler.Scheduler
}

func (c *RunPipelineCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: run_pipeline <pipeline_id> <input>")
	}
	pipelineID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline_id: %w", err)
	}
	input := strings.Join(params[1:], " ")

	pipeline, err := c.store.GetPipeline(pipelineID)
	if err != nil {
		return nil, err
	}

	job, err := c.store.CreateJob("pipeline", pipeline.WorkspaceID, pipelineID, input)
	if err != nil {
		return nil, err
	}

	// Create a cancellable context so the job can be stopped via cancel_job.
	jobCtx, cancel := context.WithCancel(context.Background())
	_ = c.store.SetJobCancel(job.ID, cancel)

	c.sched.Schedule(&scheduler.Activity{
		Job: func() {
			defer cancel()
			_ = c.store.UpdateJob(job.ID, data.JobRunning, "", "")
			result, runErr := c.runner.RunPipeline(jobCtx, pipelineID, input)
			if runErr != nil {
				_ = c.store.UpdateJob(job.ID, data.JobFailed, "", runErr.Error())
			} else {
				_ = c.store.UpdateJob(job.ID, data.JobCompleted, result, "")
			}
		},
	})

	out, _ := json.Marshal(map[string]string{"job_id": job.ID.String()})
	return &Response{Message: string(out)}, nil
}
