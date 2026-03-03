package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType identifies what happened.
type EventType string

const (
	EventAgentRunStarted   EventType = "agent_run_started"
	EventAgentRunCompleted EventType = "agent_run_completed"
	EventAgentRunFailed    EventType = "agent_run_failed"
	EventPipelineStarted   EventType = "pipeline_started"
	EventPipelineCompleted EventType = "pipeline_completed"
	EventPipelineFailed    EventType = "pipeline_failed"
)

// Event is a single audit-log entry for a workspace.
// Optional fields are omitted when empty.
type Event struct {
	Timestamp    time.Time `json:"ts"`
	WorkspaceID  string    `json:"workspace_id"`
	Type         EventType `json:"type"`
	AgentID      string    `json:"agent_id,omitempty"`
	ThreadID     string    `json:"thread_id,omitempty"`
	PipelineID   string    `json:"pipeline_id,omitempty"`
	JobID        string    `json:"job_id,omitempty"`
	LatencyMs    int64     `json:"latency_ms,omitempty"`
	InputTokens  int64     `json:"input_tokens,omitempty"`
	OutputTokens int64     `json:"output_tokens,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// Logger writes per-workspace event logs to <dataDir>/events/<workspace_id>.log.
// Each log line is a JSON-encoded Event. Append-only, never truncated.
type Logger struct {
	dir string
	mu  sync.Mutex
}

// NewLogger creates a Logger rooted at dataDir/events, creating the directory
// if it does not exist.
func NewLogger(dataDir string) (*Logger, error) {
	dir := filepath.Join(dataDir, "events")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating events directory: %w", err)
	}
	return &Logger{dir: dir}, nil
}

// Append serialises e as JSON and appends it to the workspace's event log.
// A missing WorkspaceID is a no-op.
func (l *Logger) Append(e Event) error {
	if e.WorkspaceID == "" {
		return nil
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	path := filepath.Join(l.dir, e.WorkspaceID+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening event log: %w", err)
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}
