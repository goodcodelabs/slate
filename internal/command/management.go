package command

import (
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/scheduler"
)

// SystemMetricsCommand handles: system_metrics
type SystemMetricsCommand struct {
	metrics *metrics.Metrics
}

func (c *SystemMetricsCommand) Execute(_ Context, _ []string) (*Response, error) {
	snap := c.metrics.Snapshot()
	out, _ := json.Marshal(snap)
	return &Response{Message: string(out)}, nil
}

// SystemStatsCommand handles: system_stats
type SystemStatsCommand struct {
	store   *data.Data
	sched   *scheduler.Scheduler
	metrics *metrics.Metrics
}

func (c *SystemStatsCommand) Execute(_ Context, _ []string) (*Response, error) {
	// Count jobs by status.
	jobCounts := map[string]int{
		"pending":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
	}
	jobs, _ := c.store.ListJobs(ksuid.KSUID{})
	for _, j := range jobs {
		jobCounts[string(j.Status)]++
	}

	snap := c.metrics.Snapshot()
	out, _ := json.Marshal(map[string]interface{}{
		"jobs":                jobCounts,
		"scheduler_queue":     c.sched.QueueDepth(),
		"llm_calls":           snap.LLMCalls,
		"tool_calls":          snap.ToolCalls,
		"errors":              snap.Errors,
		"input_tokens_total":  snap.InputTokensTotal,
		"output_tokens_total": snap.OutputTokensTotal,
		"active_connections":  snap.ActiveConnections,
	})
	return &Response{Message: string(out)}, nil
}

// ListJobsCommand handles: ls_jobs [workspace_id]
type ListJobsCommand struct {
	store *data.Data
}

func (c *ListJobsCommand) Execute(_ Context, params []string) (*Response, error) {
	var wsID ksuid.KSUID
	if len(params) > 0 {
		var err error
		wsID, err = ksuid.Parse(params[0])
		if err != nil {
			return nil, fmt.Errorf("invalid workspace_id: %w", err)
		}
	}

	jobs, err := c.store.ListJobs(wsID)
	if err != nil {
		return nil, err
	}

	type jobSummary struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		WorkspaceID string `json:"workspace_id"`
		PipelineID  string `json:"pipeline_id,omitempty"`
		Status      string `json:"status"`
		CreatedAt   string `json:"created_at"`
	}

	summaries := make([]jobSummary, 0, len(jobs))
	for _, j := range jobs {
		s := jobSummary{
			ID:          j.ID.String(),
			Type:        j.Type,
			WorkspaceID: j.WorkspaceID.String(),
			Status:      string(j.Status),
			CreatedAt:   j.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if j.PipelineID != (ksuid.KSUID{}) {
			s.PipelineID = j.PipelineID.String()
		}
		summaries = append(summaries, s)
	}

	out, _ := json.Marshal(summaries)
	return &Response{Message: string(out)}, nil
}

// CancelJobCommand handles: cancel_job <job_id>
type CancelJobCommand struct {
	store *data.Data
}

func (c *CancelJobCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: cancel_job <job_id>")
	}
	jobID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid job_id: %w", err)
	}
	if err := c.store.CancelJob(jobID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}
