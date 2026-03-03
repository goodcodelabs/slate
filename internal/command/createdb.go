package command

import (
	"slate/internal/data"
)

type CreateDBCommand struct {
	store *data.Data
}

func (c *CreateDBCommand) Execute(_ Context, _ []string) (*Response, error) {
	// Create a Database Record in the System Table
	return &Response{Message: "ok"}, nil
}
