package command

import (
	"encoding/json"
	"fmt"

	"slate/internal/data"

	"github.com/segmentio/ksuid"
)

// ListWorkspacesCommand handles: ls_workspaces
type ListWorkspacesCommand struct {
	store *data.Data
}

func (c *ListWorkspacesCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
	type workspaceSummary struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		CatalogID     string `json:"catalog_id,omitempty"`
		RouterAgentID string `json:"router_agent_id,omitempty"`
	}

	workspaces := c.store.ListWorkspaces()
	summaries := make([]workspaceSummary, 0, len(workspaces))
	for _, w := range workspaces {
		s := workspaceSummary{ID: w.ID.String(), Name: w.Name}
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

func (c *AddWorkspaceCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, fmt.Errorf("params must include non-empty \"name\"")
	}
	if err := c.store.AddWorkspace(p.Name); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveWorkspaceCommand struct {
	store *data.Data
}

func (c *RemoveWorkspaceCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, fmt.Errorf("params must include non-empty \"name\"")
	}
	if err := c.store.RemoveWorkspace(p.Name); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetWorkspaceCatalogCommand handles: set_workspace_catalog
type SetWorkspaceCatalogCommand struct {
	store *data.Data
}

func (c *SetWorkspaceCatalogCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		WorkspaceID string `json:"workspace_id"`
		CatalogID   string `json:"catalog_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	wsID, err := ksuid.Parse(p.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	catID, err := ksuid.Parse(p.CatalogID)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog_id: %w", err)
	}
	if err := c.store.SetWorkspaceCatalog(wsID, catID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

// SetWorkspaceRouterCommand handles: set_workspace_router
type SetWorkspaceRouterCommand struct {
	store *data.Data
}

func (c *SetWorkspaceRouterCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		WorkspaceID string `json:"workspace_id"`
		AgentID     string `json:"agent_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	wsID, err := ksuid.Parse(p.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace_id: %w", err)
	}
	agentID, err := ksuid.Parse(p.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}
	if err := c.store.SetWorkspaceRouter(wsID, agentID); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}
