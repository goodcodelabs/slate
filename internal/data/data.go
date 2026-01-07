package data

import (
	"errors"

	"github.com/segmentio/ksuid"
)

func New(name, dataDir string) (*Data, error) {
	// Initialize file store
	store, err := NewFileStore(dataDir)
	if err != nil {
		return nil, err
	}

	// Load existing data from disk
	db, err := store.Load()
	if err != nil {
		return nil, err
	}

	// Attach store to data
	db.store = store

	return db, nil
}

func (db *Data) Close() error {
	if db.store != nil {
		return db.store.Close()
	}
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

	// Persist to disk
	if db.store != nil {
		if err := db.store.SaveWorkspace(w); err != nil {
			// Rollback in-memory change on persistence failure
			delete(db.Workspaces, w.ID)
			return err
		}
	}

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

	var targetID ksuid.KSUID
	var found bool

	for _, w := range db.Workspaces {
		if w.Name == name {
			targetID = w.ID
			found = true
			break
		}
	}

	if !found {
		return errors.New("workspace not found")
	}

	// Remove from memory
	delete(db.Workspaces, targetID)

	// Persist to disk
	if db.store != nil {
		if err := db.store.DeleteWorkspace(targetID); err != nil {
			return err
		}
	}

	return nil
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

	// Persist to disk
	if db.store != nil {
		if err := db.store.SaveCatalog(c); err != nil {
			// Rollback in-memory change on persistence failure
			delete(db.Catalogs, c.ID)
			return err
		}
	}

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

	var targetID ksuid.KSUID
	var found bool

	for _, c := range db.Catalogs {
		if c.Name == name {
			targetID = c.ID
			found = true
			break
		}
	}

	if !found {
		return errors.New("catalog not found")
	}

	// Remove from memory
	delete(db.Catalogs, targetID)

	// Persist to disk
	if db.store != nil {
		if err := db.store.DeleteCatalog(targetID); err != nil {
			return err
		}
	}

	return nil
}

func (db *Data) ListCatalogs() ([]Catalog, error) {
	cs := make([]Catalog, 0, len(db.Catalogs))
	for _, c := range db.Catalogs {
		cs = append(cs, *c)
	}
	return cs, nil
}
