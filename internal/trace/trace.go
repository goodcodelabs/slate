package trace

import (
	"encoding/json"
	"time"
)

// TraceEventType identifies what happened in a thread turn.
type TraceEventType string

const (
	EventTurnStart  TraceEventType = "turn_start"
	EventLLMCall    TraceEventType = "llm_call"
	EventTextOutput TraceEventType = "text_output"
	EventThinking   TraceEventType = "thinking"
	EventToolCall   TraceEventType = "tool_call"
	EventToolResult TraceEventType = "tool_result"
	EventTurnEnd    TraceEventType = "turn_end"
)

// TraceEvent is a single entry in a thread's execution trace.
type TraceEvent struct {
	Timestamp time.Time      `json:"ts"`
	ThreadID  string         `json:"thread_id"`
	AgentID   string         `json:"agent_id,omitempty"`
	Type      TraceEventType `json:"type"`

	// llm_call
	Model        string `json:"model,omitempty"`
	Iteration    int    `json:"iteration,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	InputTokens  int64  `json:"input_tokens,omitempty"`
	OutputTokens int64  `json:"output_tokens,omitempty"`
	LatencyMs    int64  `json:"latency_ms,omitempty"`

	// tool_call
	ToolName  string          `json:"tool_name,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`

	// tool_result
	ToolOutput string `json:"tool_output,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`

	// text_output / thinking
	Text string `json:"text,omitempty"`

	// turn_end aggregates
	TotalInputTokens  int64  `json:"total_input_tokens,omitempty"`
	TotalOutputTokens int64  `json:"total_output_tokens,omitempty"`
	Error             string `json:"error,omitempty"`
}
