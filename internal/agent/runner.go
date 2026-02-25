package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
	"slate/internal/events"
	"slate/internal/llm"
	"slate/internal/metrics"
	"slate/internal/tools"
)

const (
	defaultModel     = "claude-sonnet-4-6"
	defaultMaxTokens = 1024
)

// RunnerOptions bundles optional observability dependencies for a Runner.
type RunnerOptions struct {
	Logger  *slog.Logger
	Metrics *metrics.Metrics
	Events  *events.Logger
}

// Runner executes an agent against an LLM provider, managing the agentic loop.
type Runner struct {
	provider llm.Provider
	store    *data.Data
	registry *tools.Registry
	logger   *slog.Logger
	metrics  *metrics.Metrics
	events   *events.Logger
}

// RunResult holds the output of a single agent run.
type RunResult struct {
	Response     string
	History      []llm.Message
	InputTokens  int64
	OutputTokens int64
}

func NewRunner(provider llm.Provider, store *data.Data, registry *tools.Registry, opts RunnerOptions) *Runner {
	return &Runner{
		provider: provider,
		store:    store,
		registry: registry,
		logger:   opts.Logger,
		metrics:  opts.Metrics,
		events:   opts.Events,
	}
}

// RunThread executes an agent turn within a persistent Thread, persisting
// both the user message and the assistant response to the thread's log.
func (r *Runner) RunThread(ctx context.Context, threadID ksuid.KSUID, input string) (*RunResult, error) {
	thread, err := r.store.GetThread(threadID)
	if err != nil {
		return nil, fmt.Errorf("loading thread: %w", err)
	}

	start := time.Now()
	r.emitEvent(events.Event{
		WorkspaceID: thread.WorkspaceID.String(),
		Type:        events.EventAgentRunStarted,
		AgentID:     thread.AgentID.String(),
		ThreadID:    threadID.String(),
	})

	historyLen := len(thread.Messages)

	result, err := r.Run(ctx, thread.AgentID, input, thread.Messages)
	if err != nil {
		r.emitEvent(events.Event{
			WorkspaceID: thread.WorkspaceID.String(),
			Type:        events.EventAgentRunFailed,
			AgentID:     thread.AgentID.String(),
			ThreadID:    threadID.String(),
			Error:       err.Error(),
		})
		return nil, err
	}

	r.emitEvent(events.Event{
		WorkspaceID:  thread.WorkspaceID.String(),
		Type:         events.EventAgentRunCompleted,
		AgentID:      thread.AgentID.String(),
		ThreadID:     threadID.String(),
		LatencyMs:    time.Since(start).Milliseconds(),
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	})

	// result.History = original history + user message + assistant message
	newMessages := result.History[historyLen:]
	for _, msg := range newMessages {
		if err := r.store.AppendMessage(threadID, msg); err != nil {
			return nil, fmt.Errorf("persisting message: %w", err)
		}
	}

	return result, nil
}

// RunOptions allows callers to override or augment agent settings for a single run.
type RunOptions struct {
	// SystemPromptSuffix is appended to the agent's instructions (separated by a blank line).
	SystemPromptSuffix string
}

// Run executes the agent identified by agentID against the given input.
// history is the prior conversation; pass nil for a fresh run.
func (r *Runner) Run(ctx context.Context, agentID ksuid.KSUID, input string, history []llm.Message) (*RunResult, error) {
	return r.RunWithOptions(ctx, agentID, input, history, RunOptions{})
}

// RunWithOptions is like Run but accepts per-call overrides.
func (r *Runner) RunWithOptions(ctx context.Context, agentID ksuid.KSUID, input string, history []llm.Message, opts RunOptions) (*RunResult, error) {
	agent, _, err := r.store.FindAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}

	model := agent.Model
	if model == "" {
		model = defaultModel
	}
	maxTokens := agent.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	systemPrompt := agent.Instructions
	if opts.SystemPromptSuffix != "" {
		if systemPrompt != "" {
			systemPrompt = systemPrompt + "\n\n" + opts.SystemPromptSuffix
		} else {
			systemPrompt = opts.SystemPromptSuffix
		}
	}

	// Append the new user turn to the working history.
	messages := make([]llm.Message, len(history), len(history)+8)
	copy(messages, history)
	messages = append(messages, llm.Message{
		Role: llm.RoleUser,
		Content: []llm.Content{{
			Type: llm.ContentTypeText,
			Text: input,
		}},
	})

	// Resolve the tool definitions this agent is allowed to use.
	var toolDefs []llm.ToolDef
	if r.registry != nil && len(agent.Tools) > 0 {
		toolDefs = r.registry.GetDefs(agent.Tools)
	}

	var totalInput, totalOutput int64
	iteration := 0

	for {
		callStart := time.Now()
		resp, err := r.provider.Complete(ctx, llm.CompletionRequest{
			Model:        model,
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        toolDefs,
			MaxTokens:    maxTokens,
			Temperature:  agent.Temperature,
		})
		latencyMs := time.Since(callStart).Milliseconds()

		if err != nil {
			if r.metrics != nil {
				r.metrics.RecordError()
			}
			if r.logger != nil {
				r.logger.Error("LLM completion failed",
					"agent_id", agentID,
					"model", model,
					"iteration", iteration,
					"error", err,
				)
			}
			return nil, fmt.Errorf("LLM completion: %w", err)
		}

		if r.metrics != nil {
			r.metrics.RecordLLMCall(latencyMs, resp.InputTokens, resp.OutputTokens)
		}
		if r.logger != nil {
			r.logger.Info("LLM completion",
				"agent_id", agentID,
				"model", model,
				"iteration", iteration,
				"stop_reason", resp.StopReason,
				"input_tokens", resp.InputTokens,
				"output_tokens", resp.OutputTokens,
				"latency_ms", latencyMs,
			)
		}

		totalInput += resp.InputTokens
		totalOutput += resp.OutputTokens
		messages = append(messages, resp.Message)
		iteration++

		if resp.StopReason != "tool_use" {
			break
		}

		// Execute every tool call the model requested.
		toolResults, err := r.executeToolCalls(ctx, agentID, resp.Message.Content)
		if err != nil {
			return nil, err
		}
		if len(toolResults) == 0 {
			break
		}

		// Feed results back as a user turn and continue the loop.
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: toolResults,
		})
	}

	// Extract text from the final assistant message.
	var finalText string
	if len(messages) > 0 {
		for _, c := range messages[len(messages)-1].Content {
			if c.Type == llm.ContentTypeText {
				finalText += c.Text
			}
		}
	}

	return &RunResult{
		Response:     finalText,
		History:      messages,
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
	}, nil
}

// executeToolCalls dispatches every tool_use block in content and returns
// tool_result content blocks to be appended as the next user turn.
func (r *Runner) executeToolCalls(ctx context.Context, agentID ksuid.KSUID, content []llm.Content) ([]llm.Content, error) {
	if r.registry == nil {
		return nil, nil
	}
	var results []llm.Content
	for _, c := range content {
		if c.Type != llm.ContentTypeToolUse {
			continue
		}

		toolStart := time.Now()
		output, execErr := r.registry.Execute(ctx, c.Name, c.Input)
		latencyMs := time.Since(toolStart).Milliseconds()

		if r.metrics != nil {
			r.metrics.RecordToolCall()
			if execErr != nil {
				r.metrics.RecordError()
			}
		}
		if r.logger != nil {
			if execErr != nil {
				r.logger.Error("tool execution failed",
					"agent_id", agentID,
					"tool", c.Name,
					"latency_ms", latencyMs,
					"error", execErr,
				)
			} else {
				r.logger.Info("tool executed",
					"agent_id", agentID,
					"tool", c.Name,
					"latency_ms", latencyMs,
				)
			}
		}

		result := llm.Content{
			Type:      llm.ContentTypeToolResult,
			ToolUseID: c.ID,
		}
		if execErr != nil {
			result.Output = fmt.Sprintf("error: %s", execErr.Error())
			result.IsError = true
		} else {
			result.Output = string(output)
		}
		results = append(results, result)
	}
	return results, nil
}

// emitEvent sends an event to the workspace event log, silently ignoring errors.
func (r *Runner) emitEvent(e events.Event) {
	if r.events == nil || e.WorkspaceID == "" {
		return
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	_ = r.events.Append(e)
}
