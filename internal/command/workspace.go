package command

import "slate/internal/data"

type AddWorkspaceCommand struct {
	store *data.Data
}

func (c *AddWorkspaceCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	err := c.store.AddWorkspace(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveWorkspaceCommand struct {
	store *data.Data
}

func (c *RemoveWorkspaceCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	err := c.store.RemoveWorkspace(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}
