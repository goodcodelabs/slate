package command

import "fmt"

type SynCommand struct {
}

func (c *SynCommand) Execute(connectionContext ConnectionContext, params []string) (*Response, error) {
	return &Response{ResponseMessage: fmt.Sprintf("ack")}, nil
}
