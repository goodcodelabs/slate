package parser_test

import (
	"testing"

	"slate/internal/parser"
)

func TestParseRequest(t *testing.T) {
	p := parser.New()

	tests := []struct {
		name        string
		input       string
		wantCmd     string
		wantParams  []string
		wantErr     bool
	}{
		{
			name:       "simple command no params",
			input:      "health",
			wantCmd:    "health",
			wantParams: []string{},
		},
		{
			name:       "command with one param",
			input:      "add_workspace myws",
			wantCmd:    "add_workspace",
			wantParams: []string{"myws"},
		},
		{
			name:       "command with multiple params",
			input:      "add_agent catalogid myagent",
			wantCmd:    "add_agent",
			wantParams: []string{"catalogid", "myagent"},
		},
		{
			name:       "uppercase command is lowercased",
			input:      "HEALTH",
			wantCmd:    "health",
			wantParams: []string{},
		},
		{
			name:       "mixed case command is lowercased",
			input:      "Add_Workspace foo",
			wantCmd:    "add_workspace",
			wantParams: []string{"foo"},
		},
		{
			name:       "leading and trailing whitespace trimmed",
			input:      "  health  ",
			wantCmd:    "health",
			wantParams: []string{},
		},
		{
			name:       "empty string returns empty command",
			input:      "",
			wantCmd:    "",
			wantParams: []string{},
		},
		{
			name:       "command with trailing space preserves empty param",
			input:      "set_agent_instructions agentid instructions here",
			wantCmd:    "set_agent_instructions",
			wantParams: []string{"agentid", "instructions", "here"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.ParseRequest(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Command != tc.wantCmd {
				t.Errorf("Command = %q, want %q", got.Command, tc.wantCmd)
			}
			if len(got.Params) != len(tc.wantParams) {
				t.Errorf("Params = %v, want %v", got.Params, tc.wantParams)
				return
			}
			for i, p := range tc.wantParams {
				if got.Params[i] != p {
					t.Errorf("Params[%d] = %q, want %q", i, got.Params[i], p)
				}
			}
		})
	}
}
