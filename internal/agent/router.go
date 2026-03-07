package agent

import (
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
)

// buildCatalogListing returns the catalog agent listing suffix for a workspace's system prompt.
func buildCatalogListing(r *Runner, workspace *data.Workspace) string {
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
	return sb.String()
}

