package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/scheduler"
)

// NewAgentThreadCommand handles: new_agent_thread
type NewAgentThreadCommand struct {
	store *data.Data
}

func (c *NewAgentThreadCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	name := p.Name
	if name == "" {
		name = fmt.Sprintf("thread-%s", time.Now().UTC().Format("20060102-150405"))
	}
	t, err := c.store.NewAgentThread(agentID, name)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]string{"id": t.ID.String(), "name": t.Name})
	return &Response{Message: string(out)}, nil
}

// AgentChatCommand handles: agent_chat
// Returns a job_id immediately; poll job_status / job_result for the response.
type AgentChatCommand struct {
	store  *data.Data
	runner *agent.Runner
	sched  *scheduler.Scheduler
}

func (c *AgentChatCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	threadID, err := ksuid.Parse(p.ThreadID)
	if err != nil {
		return nil, fmt.Errorf("invalid thread_id: %w", err)
	}
	if p.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	if _, err := c.store.GetAgentThread(threadID); err != nil {
		return nil, err
	}

	job, err := c.store.CreateJob("agent_thread_chat", ksuid.KSUID{}, ksuid.KSUID{}, p.Message)
	if err != nil {
		return nil, err
	}
	job.ThreadID = threadID

	jobCtx, cancel := context.WithCancel(context.Background())
	_ = c.store.SetJobCancel(job.ID, cancel)

	c.sched.Schedule(&scheduler.Activity{
		Job: func() {
			defer cancel()
			_ = c.store.UpdateJob(job.ID, data.JobRunning, "", "")
			result, runErr := c.runner.RunAgentThread(jobCtx, threadID, p.Message)
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

// ListAgentThreadsCommand handles: ls_agent_threads
type ListAgentThreadsCommand struct {
	store *data.Data
}

type agentThreadSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	Messages  int    `json:"messages"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (c *ListAgentThreadsCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
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
	threads, err := c.store.ListAgentThreads(agentID)
	if err != nil {
		return nil, err
	}

	summaries := make([]agentThreadSummary, 0, len(threads))
	for _, t := range threads {
		summaries = append(summaries, agentThreadSummary{
			ID:        t.ID.String(),
			Name:      t.Name,
			AgentID:   t.AgentID.String(),
			State:     string(t.State),
			Messages:  len(t.Messages),
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
		})
	}

	out, _ := json.Marshal(map[string]interface{}{"threads": summaries})
	return &Response{Message: string(out)}, nil
}

// AgentThreadHistoryCommand handles: agent_thread_history
type AgentThreadHistoryCommand struct {
	store *data.Data
}

func (c *AgentThreadHistoryCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	threadID, err := ksuid.Parse(p.ThreadID)
	if err != nil {
		return nil, fmt.Errorf("invalid thread_id: %w", err)
	}
	thread, err := c.store.GetAgentThread(threadID)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]interface{}{"messages": thread.Messages})
	return &Response{Message: string(out)}, nil
}
