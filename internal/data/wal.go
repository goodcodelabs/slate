package data

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/vmihailenco/msgpack/v5"
)

// OperationType represents the type of operation in the WAL
type OperationType string

const (
	OpAddWorkspace    OperationType = "ADD_WORKSPACE"
	OpRemoveWorkspace OperationType = "REMOVE_WORKSPACE"
	OpAddCatalog      OperationType = "ADD_CATALOG"
	OpRemoveCatalog   OperationType = "REMOVE_CATALOG"
	OpAddThread       OperationType = "ADD_THREAD"
	OpRemoveThread    OperationType = "REMOVE_THREAD"
	OpAddPipeline     OperationType = "ADD_PIPELINE"
	OpRemovePipeline  OperationType = "REMOVE_PIPELINE"
)

// WALEntry represents a single operation in the write-ahead log
type WALEntry struct {
	Timestamp time.Time     `json:"timestamp"`
	Sequence  uint64        `json:"sequence"`
	Operation OperationType `json:"operation"`
	EntityID  ksuid.KSUID   `json:"entity_id"`
	Data      []byte        `json:"data"` // MessagePack encoded entity data
}

// WAL manages the write-ahead log
type WAL struct {
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
	sequence uint64
	path     string
}

// NewWAL creates a new write-ahead log
func NewWAL(path string) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL file: %w", err)
	}

	wal := &WAL{
		file:     file,
		writer:   bufio.NewWriter(file),
		path:     path,
		sequence: 0,
	}

	return wal, nil
}

// Append writes an entry to the WAL
func (w *WAL) Append(op OperationType, entityID ksuid.KSUID, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.sequence++

	entry := WALEntry{
		Timestamp: time.Now().UTC(),
		Sequence:  w.sequence,
		Operation: op,
		EntityID:  entityID,
		Data:      data,
	}

	// Encode entry as JSON (one entry per line)
	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	// Write entry + newline
	if _, err := w.writer.Write(entryData); err != nil {
		return fmt.Errorf("failed to write WAL entry: %w", err)
	}
	if _, err := w.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush to disk
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush WAL: %w", err)
	}

	// Fsync for durability
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL: %w", err)
	}

	return nil
}

// Replay reads all entries from the WAL and applies them to the data
func (w *WAL) Replay(data *Data) error {
	// Open WAL for reading
	file, err := os.Open(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			// No WAL file yet, nothing to replay
			return nil
		}
		return fmt.Errorf("failed to open WAL for replay: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var maxSequence uint64

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("failed to unmarshal WAL entry: %w", err)
		}

		// Track highest sequence number
		if entry.Sequence > maxSequence {
			maxSequence = entry.Sequence
		}

		// Apply operation to data
		if err := applyWALEntry(data, &entry); err != nil {
			return fmt.Errorf("failed to apply WAL entry: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading WAL: %w", err)
	}

	// Update sequence counter
	w.sequence = maxSequence

	return nil
}

// applyWALEntry applies a single WAL entry to the in-memory data
func applyWALEntry(data *Data, entry *WALEntry) error {
	switch entry.Operation {
	case OpAddWorkspace:
		var workspace Workspace
		if err := msgpack.Unmarshal(entry.Data, &workspace); err != nil {
			return fmt.Errorf("failed to unmarshal workspace: %w", err)
		}
		data.Workspaces[workspace.ID] = &workspace

	case OpRemoveWorkspace:
		delete(data.Workspaces, entry.EntityID)

	case OpAddCatalog:
		var catalog Catalog
		if err := msgpack.Unmarshal(entry.Data, &catalog); err != nil {
			return fmt.Errorf("failed to unmarshal catalog: %w", err)
		}
		data.Catalogs[catalog.ID] = &catalog

	case OpRemoveCatalog:
		delete(data.Catalogs, entry.EntityID)

	case OpAddThread:
		var thread Thread
		if err := msgpack.Unmarshal(entry.Data, &thread); err != nil {
			return fmt.Errorf("failed to unmarshal thread: %w", err)
		}
		data.Threads[thread.ID] = &thread

	case OpRemoveThread:
		delete(data.Threads, entry.EntityID)

	case OpAddPipeline:
		var pipeline Pipeline
		if err := msgpack.Unmarshal(entry.Data, &pipeline); err != nil {
			return fmt.Errorf("failed to unmarshal pipeline: %w", err)
		}
		data.Pipelines[pipeline.ID] = &pipeline

	case OpRemovePipeline:
		delete(data.Pipelines, entry.EntityID)

	default:
		return fmt.Errorf("unknown operation type: %s", entry.Operation)
	}

	return nil
}

// Truncate removes all entries from the WAL (called after checkpoint)
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Close current file
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close WAL file: %w", err)
	}

	// Truncate file
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to truncate WAL file: %w", err)
	}

	w.file = file
	w.writer = bufio.NewWriter(file)
	w.sequence = 0

	return nil
}

// Close closes the WAL file
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush WAL on close: %w", err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close WAL file: %w", err)
	}

	return nil
}
