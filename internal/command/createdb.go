package command

import (
	"encoding/json"

	"slate/internal/data"
)

type CreateDBCommand struct {
	store *data.Data
}

func (c *CreateDBCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
	return &Response{Message: "ok"}, nil
}
