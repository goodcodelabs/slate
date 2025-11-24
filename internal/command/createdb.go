package command

import (
	"slate/internal/data"
)

type CreateDBCommand struct {
	store *data.Database
}

func (c *CreateDBCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	// Create a Database Record in the System Table
	return &Response{Message: "ok"}, nil
}
