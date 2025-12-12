package data

import "github.com/segmentio/ksuid"

func newCatalog(name string) (*Catalog, error) {
	return &Catalog{
		ID:   ksuid.New(),
		Name: name,
	}, nil
}

func (c *Catalog) AddAgent(a *Agent) error {
	c.Agents = append(c.Agents, a)

	return nil
}
