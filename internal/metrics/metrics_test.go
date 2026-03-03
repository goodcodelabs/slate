package metrics_test

import (
	"sync"
	"testing"

	"slate/internal/metrics"
)

func TestNew(t *testing.T) {
	m := metrics.New()
	s := m.Snapshot()

	if s.LLMCalls != 0 {
		t.Errorf("LLMCalls = %d, want 0", s.LLMCalls)
	}
	if s.ToolCalls != 0 {
		t.Errorf("ToolCalls = %d, want 0", s.ToolCalls)
	}
	if s.Errors != 0 {
		t.Errorf("Errors = %d, want 0", s.Errors)
	}
	if s.ActiveConnections != 0 {
		t.Errorf("ActiveConnections = %d, want 0", s.ActiveConnections)
	}
	// No LLM calls yet — latency fields should be zero.
	if s.LLMAvgLatencyMs != 0 {
		t.Errorf("LLMAvgLatencyMs = %f, want 0", s.LLMAvgLatencyMs)
	}
	if s.LLMMinLatencyMs != 0 {
		t.Errorf("LLMMinLatencyMs = %d, want 0", s.LLMMinLatencyMs)
	}
	if s.LLMMaxLatencyMs != 0 {
		t.Errorf("LLMMaxLatencyMs = %d, want 0", s.LLMMaxLatencyMs)
	}
}

func TestRecordLLMCall_Single(t *testing.T) {
	m := metrics.New()
	m.RecordLLMCall(100, 50, 30)

	s := m.Snapshot()
	if s.LLMCalls != 1 {
		t.Errorf("LLMCalls = %d, want 1", s.LLMCalls)
	}
	if s.InputTokensTotal != 50 {
		t.Errorf("InputTokensTotal = %d, want 50", s.InputTokensTotal)
	}
	if s.OutputTokensTotal != 30 {
		t.Errorf("OutputTokensTotal = %d, want 30", s.OutputTokensTotal)
	}
	if s.LLMAvgLatencyMs != 100 {
		t.Errorf("LLMAvgLatencyMs = %f, want 100", s.LLMAvgLatencyMs)
	}
	if s.LLMMinLatencyMs != 100 {
		t.Errorf("LLMMinLatencyMs = %d, want 100", s.LLMMinLatencyMs)
	}
	if s.LLMMaxLatencyMs != 100 {
		t.Errorf("LLMMaxLatencyMs = %d, want 100", s.LLMMaxLatencyMs)
	}
}

func TestRecordLLMCall_Multiple(t *testing.T) {
	m := metrics.New()
	m.RecordLLMCall(100, 10, 5)
	m.RecordLLMCall(200, 20, 10)
	m.RecordLLMCall(300, 30, 15)

	s := m.Snapshot()
	if s.LLMCalls != 3 {
		t.Errorf("LLMCalls = %d, want 3", s.LLMCalls)
	}
	if s.InputTokensTotal != 60 {
		t.Errorf("InputTokensTotal = %d, want 60", s.InputTokensTotal)
	}
	if s.OutputTokensTotal != 30 {
		t.Errorf("OutputTokensTotal = %d, want 30", s.OutputTokensTotal)
	}
	wantAvg := float64(600) / 3.0
	if s.LLMAvgLatencyMs != wantAvg {
		t.Errorf("LLMAvgLatencyMs = %f, want %f", s.LLMAvgLatencyMs, wantAvg)
	}
	if s.LLMMinLatencyMs != 100 {
		t.Errorf("LLMMinLatencyMs = %d, want 100", s.LLMMinLatencyMs)
	}
	if s.LLMMaxLatencyMs != 300 {
		t.Errorf("LLMMaxLatencyMs = %d, want 300", s.LLMMaxLatencyMs)
	}
}

func TestRecordToolCall(t *testing.T) {
	m := metrics.New()
	m.RecordToolCall()
	m.RecordToolCall()

	s := m.Snapshot()
	if s.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", s.ToolCalls)
	}
}

func TestRecordError(t *testing.T) {
	m := metrics.New()
	m.RecordError()
	m.RecordError()
	m.RecordError()

	s := m.Snapshot()
	if s.Errors != 3 {
		t.Errorf("Errors = %d, want 3", s.Errors)
	}
}

func TestConnectionCounters(t *testing.T) {
	m := metrics.New()
	m.IncrConnections()
	m.IncrConnections()
	m.IncrConnections()
	m.DecrConnections()

	s := m.Snapshot()
	if s.ActiveConnections != 2 {
		t.Errorf("ActiveConnections = %d, want 2", s.ActiveConnections)
	}
}

func TestConcurrentRecordLLMCall(t *testing.T) {
	m := metrics.New()
	const goroutines = 50

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.RecordLLMCall(10, 1, 1)
		}()
	}
	wg.Wait()

	s := m.Snapshot()
	if s.LLMCalls != goroutines {
		t.Errorf("LLMCalls = %d, want %d", s.LLMCalls, goroutines)
	}
}
