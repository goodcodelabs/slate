package command

import (
	"fmt"
)

type HealthCommand struct {
}

func (c *HealthCommand) Execute(_ Context, _ []string) (*Response, error) {
	return &Response{Message: fmt.Sprintf("ok")}, nil
}
