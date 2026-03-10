package data

import (
	"github.com/segmentio/ksuid"
	"slate/internal/llm"
)

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

	// SaveThread persists thread metadata (not messages) to disk
	SaveThread(t *Thread) error

	// DeleteThread removes a thread and its message log from disk
	DeleteThread(id ksuid.KSUID) error

	// AppendMessage appends a single message to a thread's message log
	AppendMessage(threadID ksuid.KSUID, msg llm.Message) error

	// SavePipeline persists a pipeline snapshot to disk
	SavePipeline(p *Pipeline) error

	// DeletePipeline removes a pipeline from disk
	DeletePipeline(id ksuid.KSUID) error

	// Checkpoint triggers a checkpoint operation (flush WAL, compact)
	Checkpoint() error

	// Close gracefully closes the store
	Close() error
}
