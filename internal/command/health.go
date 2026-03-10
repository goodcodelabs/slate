package command

import "encoding/json"

type HealthCommand struct{}

func (c *HealthCommand) Execute(_ Context, _ json.RawMessage) (*Response, error) {
	return &Response{Message: "ok"}, nil
}
