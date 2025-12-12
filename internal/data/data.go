package data

import (
	"errors"

	"github.com/segmentio/ksuid"
)

func New(name, dataDir string) (*Data, error) {
	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
	}

	return db, nil
}

func (db *Data) Close() error {
	return nil
}

func (db *Data) AddWorkspace(name string) error {

	if db.workspaceExists(name) {
		return errors.New("workspace already exists")
	}

	w, err := newWorkspace(name, &WorkspaceConfig{})
	if err != nil {
		return err
	}

	db.Workspaces[w.ID] = w

	return nil
}

func (db *Data) workspaceExists(name string) bool {
	// Ensure there are no name collisions
	for _, w := range db.Workspaces {
		if w.Name == name {
			return true
		}
	}

	return false
}

func (db *Data) RemoveWorkspace(name string) error {

	for _, w := range db.Workspaces {
		if w.Name == name {
			delete(db.Workspaces, w.ID)
			return nil // short circuit search
		}
	}

	return errors.New("workspace not found")
}

func (db *Data) AddCatalog(name string) error {
	if db.catalogExists(name) {
		return errors.New("catalog already exists")
	}

	c, err := newCatalog(name)
	if err != nil {
		return err
	}

	db.Catalogs[c.ID] = c

	return nil
}

func (db *Data) catalogExists(name string) bool {
	for _, c := range db.Catalogs {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (db *Data) RemoveCatalog(name string) error {

	for _, c := range db.Catalogs {
		if c.Name == name {
			delete(db.Catalogs, c.ID)
			return nil
		}
	}

	return errors.New("catalog not found")
}

func (db *Data) ListCatalogs() ([]Catalog, error) {
	cs := make([]Catalog, 0, len(db.Catalogs))
	for _, c := range db.Catalogs {
		cs = append(cs, *c)
	}
	return cs, nil
}
