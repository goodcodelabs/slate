package command

import (
	"encoding/json"
	"slate/internal/data"
)

type AddCatalogCommand struct {
	store *data.Data
}

func (c *AddCatalogCommand) Execute(_ Context, params []string) (*Response, error) {
	err := c.store.AddCatalog(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveCatalogCommand struct {
	store *data.Data
}

func (c *RemoveCatalogCommand) Execute(_ Context, params []string) (*Response, error) {
	err := c.store.RemoveCatalog(params[0])
	if err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type ListCatalogsCommand struct {
	store *data.Data
}

type ListCatalogsResponse struct {
	Catalogs []data.Catalog `json:"catalogs"`
}

func (c *ListCatalogsCommand) Execute(_ Context, _ []string) (*Response, error) {
	cs, err := c.store.ListCatalogs()
	if err != nil {
		return nil, err
	}

	response := ListCatalogsResponse{Catalogs: cs}
	jsonBytes, err := json.Marshal(response)

	return &Response{Message: string(jsonBytes)}, nil
}
