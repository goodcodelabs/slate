package command

import "slate/internal/data"

type AddCatalogCommand struct {
	store *data.Data
}

func (c *AddCatalogCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	err := c.store.AddCatalog(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveCatalogCommand struct {
	store *data.Data
}

func (c *RemoveCatalogCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	err := c.store.RemoveCatalog(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}
