package data

import "github.com/segmentio/ksuid"

func newWorkspace(name string, c *WorkspaceConfig) (*Workspace, error) {
	return &Workspace{
		ID:     ksuid.New(),
		Name:   name,
		Config: c,
	}, nil
}
