package builtin_test

import (
	"context"
	"encoding/json"
	"testing"

	"slate/internal/tools/builtin"
)

func TestShellTool_Definition(t *testing.T) {
	tool := builtin.NewShellTool()
	def := tool.Definition()

	if def.Name != "shell" {
		t.Errorf("Name = %q, want %q", def.Name, "shell")
	}
	if def.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestShellTool_Execute_SimpleCommand(t *testing.T) {
	tool := builtin.NewShellTool()
	input := json.RawMessage(`{"command":"echo hello"}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
	// echo hello\n
	if result.Output == "" {
		t.Error("output should not be empty")
	}
}

func TestShellTool_Execute_ExitCode(t *testing.T) {
	tool := builtin.NewShellTool()
	input := json.RawMessage(`{"command":"exit 42"}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("exit_code = %d, want 42", result.ExitCode)
	}
}

func TestShellTool_Execute_CombinedOutput(t *testing.T) {
	tool := builtin.NewShellTool()
	// Write to both stdout and stderr.
	input := json.RawMessage(`{"command":"echo stdout; echo stderr >&2"}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.Output == "" {
		t.Error("expected combined output, got empty")
	}
}

func TestShellTool_Execute_InvalidJSON(t *testing.T) {
	tool := builtin.NewShellTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON input, got nil")
	}
}

func TestShellTool_Execute_ZeroTimeout_DefaultsTo30s(t *testing.T) {
	// A timeout of 0 should default to 30s — just verify it runs normally.
	tool := builtin.NewShellTool()
	input := json.RawMessage(`{"command":"echo ok","timeout_seconds":0}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute with zero timeout: %v", err)
	}

	var result struct {
		ExitCode int `json:"exit_code"`
	}
	json.Unmarshal(out, &result)
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
}

func TestShellTool_Execute_ExceedMaxTimeout_DefaultsTo30s(t *testing.T) {
	// A timeout > 120 should default to 30s — just verify it runs normally.
	tool := builtin.NewShellTool()
	input := json.RawMessage(`{"command":"echo ok","timeout_seconds":999}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute with excessive timeout: %v", err)
	}

	var result struct {
		ExitCode int `json:"exit_code"`
	}
	json.Unmarshal(out, &result)
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
}
