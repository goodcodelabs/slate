package command

func InitCommand(cmd ICommand) *Command {
	return &Command{
		Cmd: cmd,
	}
}
