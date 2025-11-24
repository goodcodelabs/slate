package command

import (
	"fmt"

	"github.com/segmentio/ksuid"
)

type StartSessionCommand struct {
}

func (c *StartSessionCommand) Execute(connectionContext Context, params []string) (*Response, error) {
	return &Response{Message: fmt.Sprintf("starting_session|%s", ksuid.New().String())}, nil
}
