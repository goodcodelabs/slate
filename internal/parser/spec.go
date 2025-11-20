package parser

import "errors"

var (
	ErrUnknownCommand = errors.New("unknown command")
	ErrInvalidParams  = errors.New("invalid parameters")
)

var COMMANDS = []string{
	"set",
	"get",
	"del",
	"quit",
}
