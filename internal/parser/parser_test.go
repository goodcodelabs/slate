package parser_test

import (
	"encoding/json"
	"testing"

	"slate/internal/parser"
)

func TestParseRequest(t *testing.T) {
	p := parser.New()

	tests := []struct {
		name    string
		input   string
		wantCmd string
		wantErr bool
	}{
		{
			name:    "simple command no params",
			input:   `{"cmd":"health"}`,
			wantCmd: "health",
		},
		{
			name:    "command with params",
			input:   `{"cmd":"add_workspace","params":{"name":"myws"}}`,
			wantCmd: "add_workspace",
		},
		{
			name:    "uppercase cmd is lowercased",
			input:   `{"cmd":"HEALTH"}`,
			wantCmd: "health",
		},
		{
			name:    "empty string returns empty command",
			input:   "",
			wantCmd: "",
		},
		{
			name:    "invalid JSON returns error",
			input:   "not json",
			wantErr: true,
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
		})
	}
}

func TestParseRequest_ParamsPassedThrough(t *testing.T) {
	p := parser.New()

	req, err := p.ParseRequest(`{"cmd":"add_agent","params":{"catalog_id":"abc","name":"myagent"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var params struct {
		CatalogID string `json:"catalog_id"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}
	if params.CatalogID != "abc" {
		t.Errorf("catalog_id = %q, want %q", params.CatalogID, "abc")
	}
	if params.Name != "myagent" {
		t.Errorf("name = %q, want %q", params.Name, "myagent")
	}
}
