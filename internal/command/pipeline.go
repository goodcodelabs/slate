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

// ListPipelinesCommand handles: ls_pipelines
type ListPipelinesCommand struct {
	store *data.Data
}

func (c *ListPipelinesCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	wsID, err := ksuid.Parse(p.WorkspaceID)
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
		ID    string        `json:"id"`
		Name  string        `json:"name"`
		Steps []stepSummary `json:"steps"`
	}

	summaries := make([]pipelineSummary, 0, len(pipelines))
	for _, pl := range pipelines {
		steps := make([]stepSummary, 0, len(pl.Steps))
		for _, s := range pl.Steps {
			steps = append(steps, stepSummary{AgentID: s.AgentID.String(), Mode: string(s.Mode)})
		}
		summaries = append(summaries, pipelineSummary{ID: pl.ID.String(), Name: pl.Name, Steps: steps})
	}

	out, _ := json.Marshal(map[string]interface{}{"pipelines": summaries})
	return &Response{Message: string(out)}, nil
}

// CreatePipelineCommand handles: create_pipeline
type CreatePipelineCommand struct {
	store *data.Data
}

func (c *CreatePipelineCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	wsID, err := ksuid.Parse(p.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	pl, err := c.store.NewPipeline(wsID, p.Name)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"pipeline_id": pl.ID.String()})
	return &Response{Message: string(out)}, nil
}

// AddPipelineStepCommand handles: add_pipeline_step
type AddPipelineStepCommand struct {
	store *data.Data
}

func (c *AddPipelineStepCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		PipelineID string `json:"pipeline_id"`
		AgentID    string `json:"agent_id"`
		Mode       string `json:"mode"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	pipelineID, err := ksuid.Parse(p.PipelineID)
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline_id: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	mode := data.StepMode(p.Mode)
	if mode != data.StepModeSequential && mode != data.StepModeParallel {
		return nil, fmt.Errorf("mode must be \"sequential\" or \"parallel\"")
	}
	if err := c.store.AddPipelineStep(pipelineID, agentID, mode); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// RunPipelineCommand handles: run_pipeline
// Creates an async job and returns the job ID immediately.
type RunPipelineCommand struct {
	store  *data.Data
	runner *agent.Runner
	sched  *scheduler.Scheduler
}

func (c *RunPipelineCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		PipelineID string `json:"pipeline_id"`
		Input      string `json:"input"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	pipelineID, err := ksuid.Parse(p.PipelineID)
	if err != nil {
		return nil, fmt.Errorf("invalid pipeline_id: %w", err)
	}
	if p.Input == "" {
		return nil, fmt.Errorf("input is required")
	}

	pipeline, err := c.store.GetPipeline(pipelineID)
	if err != nil {
		return nil, err
	}

	job, err := c.store.CreateJob("pipeline", pipeline.WorkspaceID, pipelineID, p.Input)
	if err != nil {
		return nil, err
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	_ = c.store.SetJobCancel(job.ID, cancel)

	c.sched.Schedule(&scheduler.Activity{
		Job: func() {
			defer cancel()
			_ = c.store.UpdateJob(job.ID, data.JobRunning, "", "")
			result, runErr := c.runner.RunPipeline(jobCtx, pipelineID, p.Input)
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
