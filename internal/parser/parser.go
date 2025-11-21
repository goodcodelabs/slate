package parser

import (
	"log/slog"
	"os"
	"strings"
)

func New() *Parser {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	return &Parser{
		logger: logger,
	}
}

func (p *Parser) ParseRequest(request string) (*ParsedRequest, error) {
	parts := strings.Split(sanitizeRequest(request), " ")

	if len(parts) == 0 {
		return nil, ErrUnknownCommand
	}

	return &ParsedRequest{
		Command: strings.ToLower(parts[0]),
		Params:  parts[1:],
	}, nil

}

func sanitizeRequest(request string) string {
	return strings.TrimSpace(request)
}
