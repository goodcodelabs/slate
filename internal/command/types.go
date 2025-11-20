package command

import "slate/internal/data"

type ICommand interface {
	Execute(args []string) error
}

type Command struct {
	store *data.Data

	Cmd ICommand
}
