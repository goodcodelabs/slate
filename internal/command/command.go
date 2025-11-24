package command

import (
	"slate/internal/data"

	"github.com/segmentio/ksuid"
)

type Command interface {
	Execute(commandContext Context, params []string) (*Response, error)
}
type ProtocolCommand struct {
	cmd Command
}

type Response struct {
	Message string
}

type Context struct {
	IPAddress string
	SessionID ksuid.KSUID
}

func (p *ProtocolCommand) Execute(commandContext Context, params []string) (*Response, error) {
	val, err := p.cmd.Execute(commandContext, params)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func InitCommands(store *data.Database) map[string]ProtocolCommand {
	return map[string]ProtocolCommand{
		"set":           {cmd: &SetCommand{store: store}},
		"get":           {cmd: &GetCommand{store: store}},
		"del":           {cmd: &DelCommand{store: store}},
		"start_session": {cmd: &StartSessionCommand{}},
		"health":        {cmd: &HealthCommand{}},
		"syn":           {cmd: &SynCommand{}},
	}
}
