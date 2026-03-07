package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"slate/internal/agent"
	"slate/internal/data"

	"github.com/segmentio/ksuid"
)

// ListWorkspacesCommand handles: ls_workspaces
type ListWorkspacesCommand struct {
	store *data.Data
}

func (c *ListWorkspacesCommand) Execute(_ Context, _ []string) (*Response, error) {
	type workspaceSummary struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		CatalogID     string `json:"catalog_id,omitempty"`
		RouterAgentID string `json:"router_agent_id,omitempty"`
	}

	summaries := make([]workspaceSummary, 0, len(c.store.Workspaces))
	for _, w := range c.store.Workspaces {
		s := workspaceSummary{
			ID:   w.ID.String(),
			Name: w.Name,
		}
		if w.CatalogID != (ksuid.KSUID{}) {
			s.CatalogID = w.CatalogID.String()
		}
		if w.Config != nil && w.Config.RouterAgentID != (ksuid.KSUID{}) {
			s.RouterAgentID = w.Config.RouterAgentID.String()
		}
		summaries = append(summaries, s)
	}

	out, _ := json.Marshal(map[string]interface{}{"workspaces": summaries})
	return &Response{Message: string(out)}, nil
}

type AddWorkspaceCommand struct {
	store *data.Data
}

func (c *AddWorkspaceCommand) Execute(_ Context, params []string) (*Response, error) {
	err := c.store.AddWorkspace(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveWorkspaceCommand struct {
	store *data.Data
}

func (c *RemoveWorkspaceCommand) Execute(_ Context, params []string) (*Response, error) {
	err := c.store.RemoveWorkspace(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetWorkspaceCatalogCommand handles: set_workspace_catalog <workspace_id> <catalog_id>
type SetWorkspaceCatalogCommand struct {
	store *data.Data
}

func (c *SetWorkspaceCatalogCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: set_workspace_catalog <workspace_id> <catalog_id>")
	}
	wsID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	catID, err := ksuid.Parse(params[1])
	if err != nil {
		return nil, fmt.Errorf("invalid catalog_id: %w", err)
	}
	if err := c.store.SetWorkspaceCatalog(wsID, catID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetWorkspaceRouterCommand handles: set_workspace_router <workspace_id> <agent_id>
type SetWorkspaceRouterCommand struct {
	store *data.Data
}

func (c *SetWorkspaceRouterCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: set_workspace_router <workspace_id> <agent_id>")
	}
	wsID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	agentID, err := ksuid.Parse(params[1])
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.SetWorkspaceRouter(wsID, agentID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// WorkspaceChatCommand handles: workspace_chat <workspace_id> <message>
type WorkspaceChatCommand struct {
	runner *agent.Runner
}

func (c *WorkspaceChatCommand) Execute(_ Context, params []string) (*Response, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("usage: workspace_chat <workspace_id> <message>")
	}
	wsID, err := ksuid.Parse(params[0])
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	input := strings.Join(params[1:], " ")
	result, err := c.runner.RunWorkspaceChat(context.Background(), wsID, input)
	if err != nil {
		return nil, err
	}
	return &Response{Message: result.Response}, nil
}
