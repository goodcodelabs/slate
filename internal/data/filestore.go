package data

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/vmihailenco/msgpack/v5"
	"slate/internal/llm"
)

const (
	workspacesDir   = "snapshots/workspaces"
	catalogsDir     = "snapshots/catalogs"
	threadsDir      = "snapshots/threads"
	agentThreadsDir = "snapshots/agent_threads" // legacy; read-only for migration
	pipelinesDir    = "snapshots/pipelines"
	walDir          = "wal"
	walFile         = "operations.log"
	metadataFile    = "metadata.json"
)

// Metadata tracks database version and checkpoint information
type DBMetadata struct {
	Version            int       `json:"version"`
	LastCheckpoint     time.Time `json:"last_checkpoint"`
	LastCheckpointSeq  uint64    `json:"last_checkpoint_seq"`
	CheckpointInterval int       `json:"checkpoint_interval"` // number of operations
}

// FileStore implements the Store interface using file-based persistence
type FileStore struct {
	baseDir  string
	wal      *WAL
	metadata *DBMetadata
	mu       sync.RWMutex
	opCount  uint64 // operations since last checkpoint
}

// NewFileStore creates a new file-based store
func NewFileStore(baseDir string) (*FileStore, error) {
	fs := &FileStore{
		baseDir: baseDir,
		metadata: &DBMetadata{
			Version:            1,
			CheckpointInterval: 1000,
		},
	}

	if err := fs.initDirectories(); err != nil {
		return nil, err
	}

	if err := fs.loadMetadata(); err != nil {
		return nil, err
	}

	walPath := filepath.Join(baseDir, walDir, walFile)
	wal, err := NewWAL(walPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create WAL: %w", err)
	}
	fs.wal = wal

	return fs, nil
}

func (fs *FileStore) initDirectories() error {
	dirs := []string{
		filepath.Join(fs.baseDir, workspacesDir),
		filepath.Join(fs.baseDir, catalogsDir),
		filepath.Join(fs.baseDir, threadsDir),
		filepath.Join(fs.baseDir, agentThreadsDir),
		filepath.Join(fs.baseDir, pipelinesDir),
		filepath.Join(fs.baseDir, walDir),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func (fs *FileStore) loadMetadata() error {
	path := filepath.Join(fs.baseDir, metadataFile)
	fileData, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fs.saveMetadata()
		}
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	if err := json.Unmarshal(fileData, fs.metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return nil
}

func (fs *FileStore) saveMetadata() error {
	path := filepath.Join(fs.baseDir, metadataFile)
	fileData, err := json.MarshalIndent(fs.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := atomicWriteFile(path, fileData); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// Load reads all snapshots from disk and replays the WAL.
func (fs *FileStore) Load() (*Data, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	d := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
	}

	if err := fs.loadWorkspaces(d); err != nil {
		return nil, fmt.Errorf("failed to load workspaces: %w", err)
	}
	if err := fs.loadCatalogs(d); err != nil {
		return nil, fmt.Errorf("failed to load catalogs: %w", err)
	}

	// Migrate legacy agent thread snapshots into the unified threads map.
	// This is a one-time migration; subsequent loads skip already-migrated threads.
	if err := fs.migrateAgentThreadSnapshots(d); err != nil {
		return nil, fmt.Errorf("failed to migrate agent threads: %w", err)
	}

	if err := fs.loadThreadSnapshots(d); err != nil {
		return nil, fmt.Errorf("failed to load threads: %w", err)
	}
	if err := fs.loadPipelineSnapshots(d); err != nil {
		return nil, fmt.Errorf("failed to load pipelines: %w", err)
	}

	// Replay WAL on top of snapshots.
	if err := fs.wal.Replay(d); err != nil {
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	// Load message logs after WAL replay so the thread set is final.
	if err := fs.loadThreadMessages(d); err != nil {
		return nil, fmt.Errorf("failed to load thread messages: %w", err)
	}

	return d, nil
}

func (fs *FileStore) loadWorkspaces(d *Data) error {
	dir := filepath.Join(fs.baseDir, workspacesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read workspaces directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msgpack" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read workspace file %s: %w", entry.Name(), err)
		}
		var workspace Workspace
		if err := msgpack.Unmarshal(fileData, &workspace); err != nil {
			return fmt.Errorf("failed to unmarshal workspace %s: %w", entry.Name(), err)
		}
		d.Workspaces[workspace.ID] = &workspace
	}
	return nil
}

func (fs *FileStore) loadCatalogs(d *Data) error {
	dir := filepath.Join(fs.baseDir, catalogsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read catalogs directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msgpack" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read catalog file %s: %w", entry.Name(), err)
		}
		var catalog Catalog
		if err := msgpack.Unmarshal(fileData, &catalog); err != nil {
			return fmt.Errorf("failed to unmarshal catalog %s: %w", entry.Name(), err)
		}
		d.Catalogs[catalog.ID] = &catalog
	}
	return nil
}

// migrateAgentThreadSnapshots reads legacy AgentThread snapshot files and converts them
// to Thread objects in d.Threads. The new Thread snapshot and message log are written to
// threadsDir so the migration is skipped on subsequent startups.
func (fs *FileStore) migrateAgentThreadSnapshots(d *Data) error {
	dir := filepath.Join(fs.baseDir, agentThreadsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read agent_threads directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msgpack" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read agent thread file %s: %w", entry.Name(), err)
		}

		var at AgentThread
		if err := msgpack.Unmarshal(fileData, &at); err != nil {
			return fmt.Errorf("failed to unmarshal agent thread %s: %w", entry.Name(), err)
		}

		// Skip if already migrated (a Thread snapshot already exists in threadsDir).
		newSnapshotPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.msgpack", at.ID.String()))
		if _, err := os.Stat(newSnapshotPath); err == nil {
			continue
		}

		// Convert AgentThread → Thread.
		t := &Thread{
			ID:        at.ID,
			AgentID:   at.AgentID,
			Name:      at.Name,
			State:     at.State,
			CreatedAt: at.CreatedAt,
			UpdatedAt: at.UpdatedAt,
		}
		d.Threads[t.ID] = t

		// Write the new Thread snapshot.
		threadData, err := msgpack.Marshal(t)
		if err != nil {
			return fmt.Errorf("failed to marshal migrated thread %s: %w", at.ID, err)
		}
		if err := atomicWriteFile(newSnapshotPath, threadData); err != nil {
			return fmt.Errorf("failed to write migrated thread snapshot %s: %w", at.ID, err)
		}

		// Migrate message log if the new location doesn't exist yet.
		oldLogPath := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.log", at.ID.String()))
		newLogPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", at.ID.String()))
		if _, err := os.Stat(oldLogPath); err == nil {
			if _, err := os.Stat(newLogPath); os.IsNotExist(err) {
				logData, err := os.ReadFile(oldLogPath)
				if err != nil {
					return fmt.Errorf("failed to read agent thread log %s: %w", at.ID, err)
				}
				if err := os.WriteFile(newLogPath, logData, 0644); err != nil {
					return fmt.Errorf("failed to write migrated thread log %s: %w", at.ID, err)
				}
			}
		}
	}
	return nil
}

// loadThreadSnapshots loads thread metadata from snapshot files (no messages).
func (fs *FileStore) loadThreadSnapshots(d *Data) error {
	dir := filepath.Join(fs.baseDir, threadsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read threads directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msgpack" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read thread file %s: %w", entry.Name(), err)
		}
		var thread Thread
		if err := msgpack.Unmarshal(fileData, &thread); err != nil {
			return fmt.Errorf("failed to unmarshal thread %s: %w", entry.Name(), err)
		}
		d.Threads[thread.ID] = &thread
	}
	return nil
}

// loadThreadMessages reads message logs for every thread in d.Threads.
// Called after WAL replay so the thread set is final.
func (fs *FileStore) loadThreadMessages(d *Data) error {
	for id, thread := range d.Threads {
		logPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", id.String()))
		msgs, err := fs.readMessageLog(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("failed to load messages for thread %s: %w", id, err)
		}
		thread.Messages = msgs
	}
	return nil
}

// readMessageLog reads all JSON-line messages from a thread log file.
func (fs *FileStore) readMessageLog(path string) ([]llm.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var msgs []llm.Message
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg llm.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, scanner.Err()
}

// SaveWorkspace persists a workspace to both WAL and snapshot.
func (fs *FileStore) SaveWorkspace(w *Workspace) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fileData, err := msgpack.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace: %w", err)
	}
	if err := fs.wal.Append(OpAddWorkspace, w.ID, fileData); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, workspacesDir, fmt.Sprintf("%s.msgpack", w.ID.String()))
	if err := atomicWriteFile(path, fileData); err != nil {
		return fmt.Errorf("failed to write workspace snapshot: %w", err)
	}
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}
	return nil
}

// DeleteWorkspace removes a workspace from both WAL and snapshot.
func (fs *FileStore) DeleteWorkspace(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Append(OpRemoveWorkspace, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, workspacesDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove workspace snapshot: %w", err)
	}
	fs.opCount++
	return nil
}

// SaveCatalog persists a catalog to both WAL and snapshot.
func (fs *FileStore) SaveCatalog(c *Catalog) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fileData, err := msgpack.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal catalog: %w", err)
	}
	if err := fs.wal.Append(OpAddCatalog, c.ID, fileData); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, catalogsDir, fmt.Sprintf("%s.msgpack", c.ID.String()))
	if err := atomicWriteFile(path, fileData); err != nil {
		return fmt.Errorf("failed to write catalog snapshot: %w", err)
	}
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}
	return nil
}

// DeleteCatalog removes a catalog from both WAL and snapshot.
func (fs *FileStore) DeleteCatalog(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Append(OpRemoveCatalog, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, catalogsDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove catalog snapshot: %w", err)
	}
	fs.opCount++
	return nil
}

// SaveThread persists thread metadata to a snapshot file and WAL.
func (fs *FileStore) SaveThread(t *Thread) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fileData, err := msgpack.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal thread: %w", err)
	}
	if err := fs.wal.Append(OpAddThread, t.ID, fileData); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.msgpack", t.ID.String()))
	if err := atomicWriteFile(path, fileData); err != nil {
		return fmt.Errorf("failed to write thread snapshot: %w", err)
	}
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}
	return nil
}

// DeleteThread removes a thread's snapshot and message log.
func (fs *FileStore) DeleteThread(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Append(OpRemoveThread, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	snapshotPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(snapshotPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove thread snapshot: %w", err)
	}
	logPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", id.String()))
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove thread message log: %w", err)
	}
	fs.opCount++
	return nil
}

// AppendMessage appends a message to a thread's JSON-line message log.
func (fs *FileStore) AppendMessage(threadID ksuid.KSUID, msg llm.Message) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fileData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	logPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", threadID.String()))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open message log: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(fileData, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync message log: %w", err)
	}
	return nil
}

// loadPipelineSnapshots loads pipeline metadata from snapshot files.
func (fs *FileStore) loadPipelineSnapshots(d *Data) error {
	dir := filepath.Join(fs.baseDir, pipelinesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read pipelines directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msgpack" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read pipeline file %s: %w", entry.Name(), err)
		}
		var pipeline Pipeline
		if err := msgpack.Unmarshal(fileData, &pipeline); err != nil {
			return fmt.Errorf("failed to unmarshal pipeline %s: %w", entry.Name(), err)
		}
		d.Pipelines[pipeline.ID] = &pipeline
	}
	return nil
}

// SavePipeline persists a pipeline to both WAL and snapshot.
func (fs *FileStore) SavePipeline(p *Pipeline) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fileData, err := msgpack.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline: %w", err)
	}
	if err := fs.wal.Append(OpAddPipeline, p.ID, fileData); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, pipelinesDir, fmt.Sprintf("%s.msgpack", p.ID.String()))
	if err := atomicWriteFile(path, fileData); err != nil {
		return fmt.Errorf("failed to write pipeline snapshot: %w", err)
	}
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}
	return nil
}

// DeletePipeline removes a pipeline snapshot from disk.
func (fs *FileStore) DeletePipeline(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Append(OpRemovePipeline, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}
	path := filepath.Join(fs.baseDir, pipelinesDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove pipeline snapshot: %w", err)
	}
	fs.opCount++
	return nil
}

// Checkpoint flushes all data and truncates the WAL.
func (fs *FileStore) Checkpoint() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.checkpointInternal()
}

func (fs *FileStore) checkpointInternal() error {
	if err := fs.wal.Truncate(); err != nil {
		return fmt.Errorf("failed to truncate WAL: %w", err)
	}
	fs.metadata.LastCheckpoint = time.Now().UTC()
	fs.metadata.LastCheckpointSeq = fs.wal.sequence
	if err := fs.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}
	fs.opCount = 0
	return nil
}

// Close gracefully closes the file store.
func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Close(); err != nil {
		return fmt.Errorf("failed to close WAL: %w", err)
	}
	if err := fs.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}
	return nil
}

// atomicWriteFile writes data to a file atomically using a temp file + rename.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpFile = nil

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
