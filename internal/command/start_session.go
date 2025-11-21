package command

import (
	"fmt"

	"github.com/segmentio/ksuid"
)

type StartSessionCommand struct {
}

func (c *StartSessionCommand) Execute(connectionContext ConnectionContext, params []string) (*Response, error) {
	return &Response{ResponseMessage: fmt.Sprintf("starting_session|%s", ksuid.New().String())}, nil
}
