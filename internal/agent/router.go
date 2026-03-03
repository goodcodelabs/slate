package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/events"
)

// RunWorkspaceChat routes a message through the workspace's designated router agent.
// The router's system prompt is augmented with a list of catalog agents it can delegate to.
func (r *Runner) RunWorkspaceChat(ctx context.Context, workspaceID ksuid.KSUID, input string) (*RunResult, error) {
	workspace, err := r.store.GetWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("loading workspace: %w", err)
	}

	if workspace.Config == nil || workspace.Config.RouterAgentID == (ksuid.KSUID{}) {
		return nil, fmt.Errorf("workspace has no router agent configured")
	}

	// Build a catalog listing so the router knows which agents it can call.
	var sb strings.Builder
	if workspace.CatalogID != (ksuid.KSUID{}) {
		catalog, err := r.store.GetCatalog(workspace.CatalogID)
		if err == nil && len(catalog.Agents) > 0 {
			sb.WriteString("Available agents (call via the call_agent tool):\n")
			for _, a := range catalog.Agents {
				line := fmt.Sprintf("- ID: %s, Name: %s", a.ID.String(), a.Name)
				if a.Instructions != "" {
					desc := a.Instructions
					if len(desc) > 120 {
						desc = desc[:120] + "..."
					}
					line += fmt.Sprintf(", Description: %s", desc)
				}
				sb.WriteString(line + "\n")
			}
		}
	}

	routerID := workspace.Config.RouterAgentID
	wsStr := workspaceID.String()
	start := time.Now()

	r.emitEvent(events.Event{
		WorkspaceID: wsStr,
		Type:        events.EventAgentRunStarted,
		AgentID:     routerID.String(),
	})

	result, err := r.RunWithOptions(ctx, routerID, input, nil, RunOptions{
		SystemPromptSuffix: sb.String(),
	})
	if err != nil {
		r.emitEvent(events.Event{
			WorkspaceID: wsStr,
			Type:        events.EventAgentRunFailed,
			AgentID:     routerID.String(),
			Error:       err.Error(),
		})
		return nil, err
	}

	r.emitEvent(events.Event{
		WorkspaceID:  wsStr,
		Type:         events.EventAgentRunCompleted,
		AgentID:      routerID.String(),
		LatencyMs:    time.Since(start).Milliseconds(),
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	})

	return result, nil
}
