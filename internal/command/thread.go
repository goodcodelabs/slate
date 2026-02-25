package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
)

// NewThreadCommand handles: new_thread <workspace_id> <agent_id> [name...]
type NewThreadCommand struct {
	store *data.Data
}

func (c *NewThreadCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: new_thread <workspace_id> <agent_id> [name]")
	}
	workspaceID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	agentID, err := ksuid.Parse(params[1])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}

	name := strings.Join(params[2:], " ")
	if name == "" {
		name = fmt.Sprintf("thread-%s", time.Now().UTC().Format("20060102-150405"))
	}

	t, err := c.store.NewThread(workspaceID, agentID, name)
	if err != nil {
		return nil, err
	}

	out, _ := json.Marshal(map[string]string{"id": t.ID.String(), "name": t.Name})
	return &Response{Message: string(out)}, nil
}

// ChatCommand handles: chat <thread_id> <message...>
type ChatCommand struct {
	runner *agent.Runner
}

func (c *ChatCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: chat <thread_id> <message>")
	}
	threadID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid thread_id: %w", err)
	}
	input := strings.Join(params[1:], " ")

	result, err := c.runner.RunThread(context.Background(), threadID, input)
	if err != nil {
		return nil, err
	}
	return &Response{Message: result.Response}, nil
}

// ListThreadsCommand handles: ls_threads <workspace_id>
type ListThreadsCommand struct {
	store *data.Data
}

type threadSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	Messages  int    `json:"messages"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (c *ListThreadsCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: ls_threads <workspace_id>")
	}
	workspaceID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	threads, err := c.store.ListThreads(workspaceID)
	if err != nil {
		return nil, err
	}

	summaries := make([]threadSummary, 0, len(threads))
	for _, t := range threads {
		summaries = append(summaries, threadSummary{
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

// ThreadHistoryCommand handles: thread_history <thread_id>
type ThreadHistoryCommand struct {
	store *data.Data
}

func (c *ThreadHistoryCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("usage: thread_history <thread_id>")
	}
	threadID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid thread_id: %w", err)
	}
	thread, err := c.store.GetThread(threadID)
	if err != nil {
		return nil, err
	}

	out, _ := json.Marshal(map[string]interface{}{"messages": thread.Messages})
	return &Response{Message: string(out)}, nil
}

