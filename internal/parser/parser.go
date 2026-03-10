package parser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func New() *Parser {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return &Parser{logger: logger}
}

// ParseRequest parses a JSON-encoded request line.
// Expected format: {"cmd":"...", "params":{...}}
func (p *Parser) ParseRequest(line string) (*ParsedRequest, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return &ParsedRequest{Command: "", Params: json.RawMessage("{}")}, nil
	}

	var req struct {
		Cmd    string          `json:"cmd"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return nil, fmt.Errorf("invalid JSON request: %w", err)
	}

	if req.Params == nil {
		req.Params = json.RawMessage("{}")
	}

	return &ParsedRequest{
		Command: strings.ToLower(req.Cmd),
		Params:  req.Params,
	}, nil
}
