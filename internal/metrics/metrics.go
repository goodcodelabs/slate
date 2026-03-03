package metrics

import (
	"encoding/json"
	"sync"
)

// Metrics tracks aggregate runtime statistics for the server.
// All methods are safe for concurrent use.
type Metrics struct {
	mu sync.Mutex

	llmCalls          int64
	toolCalls         int64
	errors            int64
	inputTokensTotal  int64
	outputTokensTotal int64
	activeConnections int64

	llmLatencyTotal int64 // sum of LLM call latencies in ms
	llmLatencyCount int64
	llmLatencyMin   int64
	llmLatencyMax   int64
}

func New() *Metrics {
	return &Metrics{llmLatencyMin: -1}
}

func (m *Metrics) RecordLLMCall(latencyMs, inputTokens, outputTokens int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llmCalls++
	m.inputTokensTotal += inputTokens
	m.outputTokensTotal += outputTokens
	m.llmLatencyTotal += latencyMs
	m.llmLatencyCount++
	if m.llmLatencyMin < 0 || latencyMs < m.llmLatencyMin {
		m.llmLatencyMin = latencyMs
	}
	if latencyMs > m.llmLatencyMax {
		m.llmLatencyMax = latencyMs
	}
}

func (m *Metrics) RecordToolCall() {
	m.mu.Lock()
	m.toolCalls++
	m.mu.Unlock()
}

func (m *Metrics) RecordError() {
	m.mu.Lock()
	m.errors++
	m.mu.Unlock()
}

func (m *Metrics) IncrConnections() {
	m.mu.Lock()
	m.activeConnections++
	m.mu.Unlock()
}

func (m *Metrics) DecrConnections() {
	m.mu.Lock()
	m.activeConnections--
	m.mu.Unlock()
}

// Snapshot is a stable, JSON-serialisable copy of the metrics.
type Snapshot struct {
	LLMCalls          int64   `json:"llm_calls"`
	ToolCalls         int64   `json:"tool_calls"`
	Errors            int64   `json:"errors"`
	InputTokensTotal  int64   `json:"input_tokens_total"`
	OutputTokensTotal int64   `json:"output_tokens_total"`
	ActiveConnections int64   `json:"active_connections"`
	LLMAvgLatencyMs   float64 `json:"llm_avg_latency_ms"`
	LLMMinLatencyMs   int64   `json:"llm_min_latency_ms"`
	LLMMaxLatencyMs   int64   `json:"llm_max_latency_ms"`
}

func (m *Metrics) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{
		LLMCalls:          m.llmCalls,
		ToolCalls:         m.toolCalls,
		Errors:            m.errors,
		InputTokensTotal:  m.inputTokensTotal,
		OutputTokensTotal: m.outputTokensTotal,
		ActiveConnections: m.activeConnections,
	}
	if m.llmLatencyCount > 0 {
		s.LLMAvgLatencyMs = float64(m.llmLatencyTotal) / float64(m.llmLatencyCount)
		s.LLMMinLatencyMs = m.llmLatencyMin
		s.LLMMaxLatencyMs = m.llmLatencyMax
	}
	return s
}

func (m *Metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Snapshot())
}
