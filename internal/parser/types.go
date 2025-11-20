package parser

import "log/slog"

type Parser struct {
	logger *slog.Logger
}

type ParsedRequest struct {
	Command string
	Params  []string
}
