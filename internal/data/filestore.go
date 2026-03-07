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
	agentThreadsDir = "snapshots/agent_threads"
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
			CheckpointInterval: 1000, // checkpoint every 1000 operations
		},
	}

	// Create directory structure
	if err := fs.initDirectories(); err != nil {
		return nil, err
	}

	// Load metadata
	if err := fs.loadMetadata(); err != nil {
		return nil, err
	}

	// Initialize WAL
	walPath := filepath.Join(baseDir, walDir, walFile)
	wal, err := NewWAL(walPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create WAL: %w", err)
	}
	fs.wal = wal

	return fs, nil
}

// initDirectories creates the directory structure if it doesn't exist
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

// loadMetadata loads the metadata file or creates a default one
func (fs *FileStore) loadMetadata() error {
	path := filepath.Join(fs.baseDir, metadataFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// First run, save default metadata
			return fs.saveMetadata()
		}
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	if err := json.Unmarshal(data, fs.metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return nil
}

// saveMetadata persists the metadata to disk
func (fs *FileStore) saveMetadata() error {
	path := filepath.Join(fs.baseDir, metadataFile)

	data, err := json.MarshalIndent(fs.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := atomicWriteFile(path, data); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// Load reads all snapshots from disk and replays the WAL
func (fs *FileStore) Load() (*Data, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data := &Data{
		Workspaces:   make(map[ksuid.KSUID]*Workspace),
		Catalogs:     make(map[ksuid.KSUID]*Catalog),
		Threads:      make(map[ksuid.KSUID]*Thread),
		AgentThreads: make(map[ksuid.KSUID]*AgentThread),
		Pipelines:    make(map[ksuid.KSUID]*Pipeline),
	}

	// Load workspace snapshots
	if err := fs.loadWorkspaces(data); err != nil {
		return nil, fmt.Errorf("failed to load workspaces: %w", err)
	}

	// Load catalog snapshots
	if err := fs.loadCatalogs(data); err != nil {
		return nil, fmt.Errorf("failed to load catalogs: %w", err)
	}

	// Load thread metadata snapshots
	if err := fs.loadThreadSnapshots(data); err != nil {
		return nil, fmt.Errorf("failed to load threads: %w", err)
	}

	// Load agent thread metadata snapshots
	if err := fs.loadAgentThreadSnapshots(data); err != nil {
		return nil, fmt.Errorf("failed to load agent threads: %w", err)
	}

	// Load pipeline snapshots
	if err := fs.loadPipelineSnapshots(data); err != nil {
		return nil, fmt.Errorf("failed to load pipelines: %w", err)
	}

	// Replay WAL to get latest state (may add/remove threads, pipelines)
	if err := fs.wal.Replay(data); err != nil {
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	// Load message logs for all threads (after WAL replay so the thread set is final)
	if err := fs.loadThreadMessages(data); err != nil {
		return nil, fmt.Errorf("failed to load thread messages: %w", err)
	}

	// Load message logs for all agent threads
	if err := fs.loadAgentThreadMessages(data); err != nil {
		return nil, fmt.Errorf("failed to load agent thread messages: %w", err)
	}

	return data, nil
}

// loadWorkspaces loads all workspace snapshot files
func (fs *FileStore) loadWorkspaces(data *Data) error {
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

		data.Workspaces[workspace.ID] = &workspace
	}

	return nil
}

// loadCatalogs loads all catalog snapshot files
func (fs *FileStore) loadCatalogs(data *Data) error {
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

		data.Catalogs[catalog.ID] = &catalog
	}

	return nil
}

// SaveWorkspace persists a workspace to both WAL and snapshot
func (fs *FileStore) SaveWorkspace(w *Workspace) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Marshal workspace
	data, err := msgpack.Marshal(w)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace: %w", err)
	}

	// Append to WAL first (durability)
	if err := fs.wal.Append(OpAddWorkspace, w.ID, data); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	// Write snapshot file
	path := filepath.Join(fs.baseDir, workspacesDir, fmt.Sprintf("%s.msgpack", w.ID.String()))
	if err := atomicWriteFile(path, data); err != nil {
		return fmt.Errorf("failed to write workspace snapshot: %w", err)
	}

	// Check if we need to checkpoint
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}

	return nil
}

// SaveCatalog persists a catalog to both WAL and snapshot
func (fs *FileStore) SaveCatalog(c *Catalog) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Marshal catalog
	data, err := msgpack.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal catalog: %w", err)
	}

	// Append to WAL first (durability)
	if err := fs.wal.Append(OpAddCatalog, c.ID, data); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	// Write snapshot file
	path := filepath.Join(fs.baseDir, catalogsDir, fmt.Sprintf("%s.msgpack", c.ID.String()))
	if err := atomicWriteFile(path, data); err != nil {
		return fmt.Errorf("failed to write catalog snapshot: %w", err)
	}

	// Check if we need to checkpoint
	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}

	return nil
}

// DeleteWorkspace removes a workspace from both WAL and snapshot
func (fs *FileStore) DeleteWorkspace(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Append to WAL
	if err := fs.wal.Append(OpRemoveWorkspace, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	// Remove snapshot file
	path := filepath.Join(fs.baseDir, workspacesDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove workspace snapshot: %w", err)
	}

	fs.opCount++
	return nil
}

// DeleteCatalog removes a catalog from both WAL and snapshot
func (fs *FileStore) DeleteCatalog(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Append to WAL
	if err := fs.wal.Append(OpRemoveCatalog, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	// Remove snapshot file
	path := filepath.Join(fs.baseDir, catalogsDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove catalog snapshot: %w", err)
	}

	fs.opCount++
	return nil
}

// Checkpoint flushes all data and truncates the WAL
func (fs *FileStore) Checkpoint() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.checkpointInternal()
}

// checkpointInternal performs the checkpoint operation (caller must hold lock)
func (fs *FileStore) checkpointInternal() error {
	// Truncate WAL
	if err := fs.wal.Truncate(); err != nil {
		return fmt.Errorf("failed to truncate WAL: %w", err)
	}

	// Update metadata
	fs.metadata.LastCheckpoint = time.Now().UTC()
	fs.metadata.LastCheckpointSeq = fs.wal.sequence
	if err := fs.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	fs.opCount = 0
	return nil
}

// Close gracefully closes the file store
func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Close WAL
	if err := fs.wal.Close(); err != nil {
		return fmt.Errorf("failed to close WAL: %w", err)
	}

	// Save metadata
	if err := fs.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	return nil
}

// loadThreadSnapshots loads thread metadata from snapshot files (no messages).
func (fs *FileStore) loadThreadSnapshots(data *Data) error {
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

		data.Threads[thread.ID] = &thread
	}

	return nil
}

// loadThreadMessages reads message logs for every thread in data.Threads.
// Called after WAL replay so the thread set is final.
func (fs *FileStore) loadThreadMessages(data *Data) error {
	for id, thread := range data.Threads {
		logPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", id.String()))
		msgs, err := fs.readMessageLog(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // no messages yet
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
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MB per line
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

// SaveThread persists thread metadata to a snapshot file and WAL.
// Message history is not included (stored separately in the message log).
func (fs *FileStore) SaveThread(t *Thread) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := msgpack.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal thread: %w", err)
	}

	if err := fs.wal.Append(OpAddThread, t.ID, data); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	path := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.msgpack", t.ID.String()))
	if err := atomicWriteFile(path, data); err != nil {
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

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	logPath := filepath.Join(fs.baseDir, threadsDir, fmt.Sprintf("%s.log", threadID.String()))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open message log: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync message log: %w", err)
	}

	return nil
}

// loadAgentThreadSnapshots loads agent thread metadata from snapshot files (no messages).
func (fs *FileStore) loadAgentThreadSnapshots(data *Data) error {
	dir := filepath.Join(fs.baseDir, agentThreadsDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
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

		var thread AgentThread
		if err := msgpack.Unmarshal(fileData, &thread); err != nil {
			return fmt.Errorf("failed to unmarshal agent thread %s: %w", entry.Name(), err)
		}

		data.AgentThreads[thread.ID] = &thread
	}

	return nil
}

// loadAgentThreadMessages reads message logs for every agent thread in data.AgentThreads.
// Called after WAL replay so the thread set is final.
func (fs *FileStore) loadAgentThreadMessages(data *Data) error {
	for id, thread := range data.AgentThreads {
		logPath := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.log", id.String()))
		msgs, err := fs.readMessageLog(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // no messages yet
			}
			return fmt.Errorf("failed to load messages for agent thread %s: %w", id, err)
		}
		thread.Messages = msgs
	}
	return nil
}

// SaveAgentThread persists agent thread metadata to WAL + snapshot file.
// Message history is not included (stored separately in the message log).
func (fs *FileStore) SaveAgentThread(t *AgentThread) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := msgpack.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to marshal agent thread: %w", err)
	}

	if err := fs.wal.Append(OpAddAgentThread, t.ID, data); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	path := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.msgpack", t.ID.String()))
	if err := atomicWriteFile(path, data); err != nil {
		return fmt.Errorf("failed to write agent thread snapshot: %w", err)
	}

	fs.opCount++
	if fs.opCount >= uint64(fs.metadata.CheckpointInterval) {
		if err := fs.checkpointInternal(); err != nil {
			fmt.Printf("Warning: checkpoint failed: %v\n", err)
		}
	}

	return nil
}

// DeleteAgentThread removes an agent thread's snapshot and message log.
func (fs *FileStore) DeleteAgentThread(id ksuid.KSUID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.wal.Append(OpRemoveAgentThread, id, nil); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	snapshotPath := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.msgpack", id.String()))
	if err := os.Remove(snapshotPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove agent thread snapshot: %w", err)
	}

	logPath := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.log", id.String()))
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove agent thread message log: %w", err)
	}

	fs.opCount++
	return nil
}

// AppendAgentMessage appends a message to an agent thread's JSON-line message log.
func (fs *FileStore) AppendAgentMessage(threadID ksuid.KSUID, msg llm.Message) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	logPath := filepath.Join(fs.baseDir, agentThreadsDir, fmt.Sprintf("%s.log", threadID.String()))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open agent message log: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync agent message log: %w", err)
	}

	return nil
}

// loadPipelineSnapshots loads pipeline metadata from snapshot files.
func (fs *FileStore) loadPipelineSnapshots(data *Data) error {
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

		data.Pipelines[pipeline.ID] = &pipeline
	}

	return nil
}

// SavePipeline persists a pipeline to both WAL and snapshot.
func (fs *FileStore) SavePipeline(p *Pipeline) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := msgpack.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline: %w", err)
	}

	if err := fs.wal.Append(OpAddPipeline, p.ID, data); err != nil {
		return fmt.Errorf("failed to append to WAL: %w", err)
	}

	path := filepath.Join(fs.baseDir, pipelinesDir, fmt.Sprintf("%s.msgpack", p.ID.String()))
	if err := atomicWriteFile(path, data); err != nil {
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

// atomicWriteFile writes data to a file atomically using a temp file + rename
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpFile = nil // Prevent cleanup in defer

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
