package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
)

// JobStatusCommand handles: job_status
type JobStatusCommand struct {
	store *data.Data
}

func (c *JobStatusCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	jobID, err := ksuid.Parse(p.JobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job_id: %w", err)
	}
	job, err := c.store.GetJob(jobID)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]interface{}{
		"status":       job.Status,
		"created_at":   job.CreatedAt,
		"started_at":   job.StartedAt,
		"completed_at": job.CompletedAt,
	})
	return &Response{Message: string(out)}, nil
}

// JobResultCommand handles: job_result
type JobResultCommand struct {
	store *data.Data
}

func (c *JobResultCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	jobID, err := ksuid.Parse(p.JobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job_id: %w", err)
	}
	job, err := c.store.GetJob(jobID)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]interface{}{
		"status": job.Status,
		"result": job.Result,
		"error":  job.Error,
	})
	return &Response{Message: string(out)}, nil
}

// WaitJobCommand handles: wait_job
// Blocks until the job reaches a terminal state (completed or failed).
type WaitJobCommand struct {
	store *data.Data
}

func (c *WaitJobCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	jobID, err := ksuid.Parse(p.JobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job_id: %w", err)
	}

	const (
		pollInterval = 100 * time.Millisecond
		maxWait      = 10 * time.Minute
	)
	deadline := time.Now().Add(maxWait)

	for {
		job, err := c.store.GetJob(jobID)
		if err != nil {
			return nil, err
		}
		if job.Status == data.JobCompleted || job.Status == data.JobFailed {
			out, _ := json.Marshal(map[string]interface{}{
				"status": job.Status,
				"result": job.Result,
				"error":  job.Error,
			})
			return &Response{Message: string(out)}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for job %s", jobID)
		}
		time.Sleep(pollInterval)
	}
}
