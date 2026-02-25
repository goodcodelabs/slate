package command

import (
	"encoding/json"
	"fmt"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
)

// JobStatusCommand handles: job_status <job_id>
type JobStatusCommand struct {
	store *data.Data
}

func (c *JobStatusCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: job_status <job_id>")
	}
	jobID, err := ksuid.Parse(params[0])
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

// JobResultCommand handles: job_result <job_id>
type JobResultCommand struct {
	store *data.Data
}

func (c *JobResultCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: job_result <job_id>")
	}
	jobID, err := ksuid.Parse(params[0])
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
