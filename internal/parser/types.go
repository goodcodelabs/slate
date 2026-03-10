package parser

import (
	"encoding/json"
	"log/slog"
)

type Parser struct {
	logger *slog.Logger
}

type ParsedRequest struct {
	Command string
	Params  json.RawMessage
}
