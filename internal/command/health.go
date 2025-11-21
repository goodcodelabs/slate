package command

import (
	"fmt"
)

type HealthCommand struct {
}

func (c *HealthCommand) Execute(connectionContext ConnectionContext, params []string) (*Response, error) {
	return &Response{ResponseMessage: fmt.Sprintf("health check")}, nil
}
