package command

import (
	"fmt"
)

type HealthCommand struct {
}

func (c *HealthCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	return &Response{Message: fmt.Sprintf("health check")}, nil
}
