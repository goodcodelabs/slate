package llm

import "context"

// Provider is the interface every LLM backend must implement.
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}
