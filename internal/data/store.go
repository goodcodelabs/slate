package data

import "github.com/segmentio/ksuid"

// Store defines the interface for persisting data to disk
type Store interface {
	// Load loads all data from disk into memory
	Load() (*Data, error)

	// SaveWorkspace persists a single workspace to disk
	SaveWorkspace(w *Workspace) error

	// SaveCatalog persists a single catalog to disk
	SaveCatalog(c *Catalog) error

	// DeleteWorkspace removes a workspace from disk
	DeleteWorkspace(id ksuid.KSUID) error

	// DeleteCatalog removes a catalog from disk
	DeleteCatalog(id ksuid.KSUID) error

	// Checkpoint triggers a checkpoint operation (flush WAL, compact)
	Checkpoint() error

	// Close gracefully closes the store
	Close() error
}
