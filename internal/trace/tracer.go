package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Tracer writes per-thread execution trace logs to <dataDir>/traces/<thread_id>.jsonl.
// Each log line is a JSON-encoded TraceEvent. Append-only, never truncated.
type Tracer struct {
	dir string
	mu  sync.Mutex
}

// NewTracer creates a Tracer rooted at dataDir/traces, creating the directory
// if it does not exist.
func NewTracer(dataDir string) (*Tracer, error) {
	dir := filepath.Join(dataDir, "traces")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating traces directory: %w", err)
	}
	return &Tracer{dir: dir}, nil
}

// Append serialises e as JSON and appends it to the thread's trace log.
// No-op when the receiver is nil.
func (t *Tracer) Append(e TraceEvent) error {
	if t == nil {
		return nil
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling trace event: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	path := filepath.Join(t.dir, e.ThreadID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening trace log: %w", err)
	}
	defer f.Close()

	_, err = f.Write(append(data, '\n'))
	return err
}

// Read returns all trace events for the given thread ID.
// If limit > 0, only the last limit events are returned.
// Returns an empty slice (not an error) if no trace file exists.
// No-op when the receiver is nil — returns empty slice.
func (t *Tracer) Read(threadID string, limit int) ([]TraceEvent, error) {
	if t == nil {
		return []TraceEvent{}, nil
	}

	path := filepath.Join(t.dir, threadID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceEvent{}, nil
		}
		return nil, fmt.Errorf("opening trace log: %w", err)
	}
	defer f.Close()

	var events []TraceEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e TraceEvent
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parsing trace event: %w", err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading trace log: %w", err)
	}

	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	if events == nil {
		events = []TraceEvent{}
	}
	return events, nil
}
