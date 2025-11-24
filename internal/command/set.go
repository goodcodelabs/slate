package command

import (
	"slate/internal/data"
	"strings"
)

type SetCommand struct {
	store *data.Database
}

func (c *SetCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	err := c.store.Set(params[0], strings.Join(params[1:], " "))
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}
