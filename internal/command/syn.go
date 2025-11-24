package command

import "fmt"

type SynCommand struct {
}

func (c *SynCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	return &Response{Message: fmt.Sprintf("ack")}, nil
}
