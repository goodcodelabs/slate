package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"slate/internal/llm"
	"slate/internal/tools"
)

// fakeTool is a simple Tool implementation for testing.
type fakeTool struct {
	name   string
	output string
}

func (f *fakeTool) Definition() llm.ToolDef {
	return llm.ToolDef{
		Name:        f.name,
		Description: "fake tool: " + f.name,
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"result":"` + f.output + `"}`), nil
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := tools.NewRegistry()
	tool := &fakeTool{name: "my_tool", output: "hello"}
	r.Register(tool)

	got, ok := r.Get("my_tool")
	if !ok {
		t.Fatal("expected tool to be found after register")
	}
	if got.Definition().Name != "my_tool" {
		t.Errorf("tool name = %q, want %q", got.Definition().Name, "my_tool")
	}
}

func TestRegistry_Get_Missing(t *testing.T) {
	r := tools.NewRegistry()
	_, ok := r.Get("missing")
	if ok {
		t.Fatal("expected tool not found for missing name")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "tool_a"})
	r.Register(&fakeTool{name: "tool_b"})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["tool_a"] || !nameSet["tool_b"] {
		t.Errorf("Names did not include both tools: %v", names)
	}
}

func TestRegistry_GetDefs_SubsetAndMissing(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "t1"})
	r.Register(&fakeTool{name: "t2"})

	// Request t1 and a missing tool — only t1 should be returned.
	defs := r.GetDefs([]string{"t1", "missing"})
	if len(defs) != 1 {
		t.Errorf("GetDefs: expected 1 def, got %d", len(defs))
	}
	if defs[0].Name != "t1" {
		t.Errorf("GetDefs: expected t1, got %s", defs[0].Name)
	}
}

func TestRegistry_Execute_Success(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "exec_tool", output: "world"})

	out, err := r.Execute(context.Background(), "exec_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(out) != `{"result":"world"}` {
		t.Errorf("Execute output = %s, want %s", string(out), `{"result":"world"}`)
	}
}

func TestRegistry_Execute_UnknownTool(t *testing.T) {
	r := tools.NewRegistry()
	_, err := r.Execute(context.Background(), "no_such_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}
