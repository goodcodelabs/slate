package llm

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// Content represents a single block within a message.
type Content struct {
	Type ContentType `json:"type"`

	// text fields
	Text string `json:"text,omitempty"`

	// tool_use fields (assistant → orchestrator)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result fields (orchestrator → assistant)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Output    string `json:"output,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role    Role      `json:"role"`
	Content []Content `json:"content"`
}

// ToolDef describes a tool the agent can invoke.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema object
}

// CompletionRequest is the provider-agnostic input to a single LLM call.
type CompletionRequest struct {
	Model        string
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float64
}

// CompletionResponse is the provider-agnostic output of a single LLM call.
type CompletionResponse struct {
	Message      Message
	StopReason   string
	InputTokens  int64
	OutputTokens int64
}
