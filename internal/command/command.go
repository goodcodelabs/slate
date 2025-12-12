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

func InitCommands(store *data.Data) map[string]ProtocolCommand {
	return map[string]ProtocolCommand{
		"add_workspace": {cmd: &AddWorkspaceCommand{store: store}},
		"del_workspace": {cmd: &RemoveWorkspaceCommand{store: store}},
		"add_catalog":   {cmd: &AddCatalogCommand{store: store}},
		"del_catalog":   {cmd: &RemoveCatalogCommand{store: store}},
		"ls_catalogs":   {cmd: &ListCatalogsCommand{store: store}},
		"health":        {cmd: &HealthCommand{}},
	}
}
