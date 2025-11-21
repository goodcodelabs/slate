package command

import "slate/internal/data"

type GetCommand struct {
	store *data.Database
}

func (c *GetCommand) Execute(connectionContext ConnectionContext, params []string) (*Response, error) {
	val, err := c.store.Get(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{ResponseMessage: val}, nil
}
