package trace

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTracer_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTracer(dir)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	threadID := "testthread001"
	events := []TraceEvent{
		{Timestamp: time.Now().UTC(), ThreadID: threadID, Type: EventTurnStart},
		{Timestamp: time.Now().UTC(), ThreadID: threadID, Type: EventLLMCall, Model: "claude-sonnet-4-6", Iteration: 0, StopReason: "end_turn", InputTokens: 100, OutputTokens: 50, LatencyMs: 500},
		{Timestamp: time.Now().UTC(), ThreadID: threadID, Type: EventTextOutput, Text: "hello"},
		{Timestamp: time.Now().UTC(), ThreadID: threadID, Type: EventTurnEnd, TotalInputTokens: 100, TotalOutputTokens: 50},
	}

	for _, e := range events {
		if err := tr.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := tr.Read(threadID, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("Read returned %d events, want %d", len(got), len(events))
	}
	for i, e := range got {
		if e.Type != events[i].Type {
			t.Errorf("event[%d].Type = %q, want %q", i, e.Type, events[i].Type)
		}
	}
}

func TestTracer_ReadWithLimit(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTracer(dir)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	threadID := "limitthread"
	for i := 0; i < 5; i++ {
		_ = tr.Append(TraceEvent{ThreadID: threadID, Type: EventTextOutput, Text: "msg"})
	}

	got, err := tr.Read(threadID, 2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Read(limit=2) returned %d events, want 2", len(got))
	}
}

func TestTracer_ReadMissingFile(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTracer(dir)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	got, err := tr.Read("nonexistent-thread", 0)
	if err != nil {
		t.Fatalf("Read on missing file returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d events", len(got))
	}
}

func TestTracer_NilReceiver(t *testing.T) {
	var tr *Tracer

	if err := tr.Append(TraceEvent{ThreadID: "x", Type: EventTurnStart}); err != nil {
		t.Errorf("nil Append should be no-op, got error: %v", err)
	}

	got, err := tr.Read("x", 0)
	if err != nil {
		t.Errorf("nil Read should return empty, got error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("nil Read should return empty slice, got %d events", len(got))
	}
}

func TestTracer_MultipleAppends_Order(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTracer(dir)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	threadID := "orderthread"
	types := []TraceEventType{EventTurnStart, EventLLMCall, EventToolCall, EventToolResult, EventTurnEnd}
	for _, et := range types {
		_ = tr.Append(TraceEvent{ThreadID: threadID, Type: et})
	}

	got, err := tr.Read(threadID, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(types) {
		t.Fatalf("got %d events, want %d", len(got), len(types))
	}
	for i, et := range types {
		if got[i].Type != et {
			t.Errorf("event[%d].Type = %q, want %q", i, got[i].Type, et)
		}
	}
}

func TestTracer_ToolCallInput_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	tr, err := NewTracer(dir)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	threadID := "toolthread"
	input := json.RawMessage(`{"command":"ls -la"}`)
	_ = tr.Append(TraceEvent{
		ThreadID:  threadID,
		Type:      EventToolCall,
		ToolName:  "shell",
		ToolUseID: "toolu_01abc",
		ToolInput: input,
	})

	got, err := tr.Read(threadID, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ToolName != "shell" {
		t.Errorf("ToolName = %q, want %q", got[0].ToolName, "shell")
	}
	if string(got[0].ToolInput) != string(input) {
		t.Errorf("ToolInput = %s, want %s", got[0].ToolInput, input)
	}
}
