package command

import "slate/internal/data"

type Set Command

func InitSetCommand(store *data.Data) *Set {
	return &Set{
		store: store,
	}
}

func (s *Set) Execute(args []string) error {
	err := s.store.Set(args[0], args[1])
	if err != nil {
		return err
	}
	return nil
}
