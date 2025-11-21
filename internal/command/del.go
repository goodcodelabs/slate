package command

import (
	"slate/internal/data"
)

type DelCommand struct {
	store *data.Database
}

func (c *DelCommand) Execute(connectionContext ConnectionContext, params []string) (*Response, error) {
	err := c.store.Del(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{ResponseMessage: "ok"}, nil
}
