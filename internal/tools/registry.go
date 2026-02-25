package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"slate/internal/llm"
)

// Registry holds the set of available tools and dispatches execution.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry, keyed by its definition name.
func (r *Registry) Register(t Tool) {
	r.tools[t.Definition().Name] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetDefs returns ToolDef values for a subset of named tools.
// Names that are not registered are silently skipped.
func (r *Registry) GetDefs(names []string) []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(names))
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			defs = append(defs, t.Definition())
		}
	}
	return defs
}

// Execute dispatches a tool call by name and returns JSON output.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}
