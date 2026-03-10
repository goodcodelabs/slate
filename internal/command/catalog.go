package command

import (
	"encoding/json"
	"fmt"

	"slate/internal/data"
)

type AddCatalogCommand struct {
	store *data.Data
}

func (c *AddCatalogCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, fmt.Errorf("params must include non-empty \"name\"")
	}
	if err := c.store.AddCatalog(p.Name); err != nil {
		return nil, err
	}
	return &Response{Message: "ok"}, nil
}

type RemoveCatalogCommand struct {
	store *data.Data
}

func (c *RemoveCatalogCommand) Execute(_ Context, params json.RawMessage) (*Response, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, fmt.Errorf("params must include non-empty \"name\"")
	}
	if err := c.store.RemoveCatalog(p.Name); err != nil {
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

func (c *ListCatalogsCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
	cs, err := c.store.ListCatalogs()
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(ListCatalogsResponse{Catalogs: cs})
	return &Response{Message: string(out)}, nil
}
