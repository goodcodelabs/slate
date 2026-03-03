package events_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"slate/internal/events"
)

func TestNewLogger_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	_ = logger

	eventsDir := filepath.Join(dir, "events")
	if _, err := os.Stat(eventsDir); os.IsNotExist(err) {
		t.Errorf("events directory was not created at %s", eventsDir)
	}
}

func TestAppend_NoWorkspaceID_NoOp(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	err = logger.Append(events.Event{
		Type: events.EventAgentRunStarted,
		// WorkspaceID intentionally empty
	})
	if err != nil {
		t.Fatalf("Append with empty WorkspaceID should be a no-op, got error: %v", err)
	}

	// No log file should be created.
	eventsDir := filepath.Join(dir, "events")
	entries, _ := os.ReadDir(eventsDir)
	if len(entries) != 0 {
		t.Errorf("expected no log files, got %d", len(entries))
	}
}

func TestAppend_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	wsID := "testworkspace123"
	ev := events.Event{
		WorkspaceID: wsID,
		Type:        events.EventAgentRunCompleted,
		AgentID:     "agent-abc",
		LatencyMs:   42,
	}

	if err := logger.Append(ev); err != nil {
		t.Fatalf("Append: %v", err)
	}

	logPath := filepath.Join(dir, "events", wsID+".log")
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("opening log file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected at least one line in log file")
	}

	var got events.Event
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("parsing log line: %v", err)
	}

	if got.WorkspaceID != wsID {
		t.Errorf("WorkspaceID = %q, want %q", got.WorkspaceID, wsID)
	}
	if got.Type != events.EventAgentRunCompleted {
		t.Errorf("Type = %q, want %q", got.Type, events.EventAgentRunCompleted)
	}
	if got.AgentID != "agent-abc" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-abc")
	}
	if got.LatencyMs != 42 {
		t.Errorf("LatencyMs = %d, want 42", got.LatencyMs)
	}
}

func TestAppend_AutoFillsTimestamp(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	before := time.Now().UTC()
	err = logger.Append(events.Event{
		WorkspaceID: "ws1",
		Type:        events.EventAgentRunStarted,
		// Timestamp intentionally zero
	})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	logPath := filepath.Join(dir, "events", "ws1.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	var got events.Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing log: %v", err)
	}

	if got.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if got.Timestamp.Before(before) || got.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range [%v, %v]", got.Timestamp, before, after)
	}
}

func TestAppend_MultipleEvents_AppendToSameFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	wsID := "ws-multi"
	for i := 0; i < 3; i++ {
		if err := logger.Append(events.Event{
			WorkspaceID: wsID,
			Type:        events.EventAgentRunStarted,
		}); err != nil {
			t.Fatalf("Append[%d]: %v", i, err)
		}
	}

	logPath := filepath.Join(dir, "events", wsID+".log")
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 log lines, got %d", count)
	}
}

func TestAppend_SeparateFilesPerWorkspace(t *testing.T) {
	dir := t.TempDir()
	logger, err := events.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	workspaces := []string{"ws-a", "ws-b", "ws-c"}
	for _, ws := range workspaces {
		if err := logger.Append(events.Event{
			WorkspaceID: ws,
			Type:        events.EventAgentRunStarted,
		}); err != nil {
			t.Fatalf("Append for %s: %v", ws, err)
		}
	}

	eventsDir := filepath.Join(dir, "events")
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		t.Fatalf("reading events dir: %v", err)
	}
	if len(entries) != len(workspaces) {
		t.Errorf("expected %d log files, got %d", len(workspaces), len(entries))
	}
}
