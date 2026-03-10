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

func (c *SystemMetricsCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
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

func (c *SystemStatsCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
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

// ListJobsCommand handles: ls_jobs
type ListJobsCommand struct {
	store *data.Data
}

func (c *ListJobsCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		WorkspaceID string `json:"workspace_id"`
	}
	// workspace_id is optional; ignore unmarshal errors for empty params
	_ = json.Unmarshal(params, &p)

	var wsID ksuid.KSUID
	if p.WorkspaceID != "" {
		var err error
		wsID, err = ksuid.Parse(p.WorkspaceID)
		if err != nil {
			return nil, errorResponse("invalid workspace_id")
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
		ThreadID    string `json:"thread_id,omitempty"`
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
		if j.ThreadID != (ksuid.KSUID{}) {
			s.ThreadID = j.ThreadID.String()
		}
		summaries = append(summaries, s)
	}

	out, _ := json.Marshal(summaries)
	return &Response{Message: string(out)}, nil
}

// CancelJobCommand handles: cancel_job
type CancelJobCommand struct {
	store *data.Data
}

func (c *CancelJobCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errorResponse("invalid params")
	}
	jobID, err := ksuid.Parse(p.JobID)
	if err != nil {
		return nil, errorResponse("invalid job_id")
	}
	if err := c.store.CancelJob(jobID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

func errorResponse(msg string) error {
	return fmt.Errorf("%s", msg)
}
