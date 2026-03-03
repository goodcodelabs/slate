package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/llm"
	"slate/internal/tools"
)

// fakeProvider implements llm.Provider for testing.
type fakeProvider struct {
	responses []*llm.CompletionResponse
	callIdx   int
	err       error
}

func (f *fakeProvider) Complete(_ context.Context, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.callIdx < len(f.responses) {
		r := f.responses[f.callIdx]
		f.callIdx++
		return r, nil
	}
	// Last response repeats.
	if len(f.responses) > 0 {
		return f.responses[len(f.responses)-1], nil
	}
	return nil, fmt.Errorf("fakeProvider: no responses configured")
}

func textResponse(text string) *llm.CompletionResponse {
	return &llm.CompletionResponse{
		Message: llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.Content{{
				Type: llm.ContentTypeText,
				Text: text,
			}},
		},
		StopReason:   "end_turn",
		InputTokens:  10,
		OutputTokens: 5,
	}
}

func newRunnerDB(t *testing.T, p llm.Provider) (*agent.Runner, *data.Data) {
	t.Helper()
	db, err := data.New("test", t.TempDir())
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	registry := tools.NewRegistry()
	runner := agent.NewRunner(p, db, registry, agent.RunnerOptions{})
	return runner, db
}

// addTestAgent creates a catalog+agent in db and returns the agent ID.
func addTestAgent(t *testing.T, db *data.Data) ksuid.KSUID {
	t.Helper()
	if err := db.AddCatalog("test-cat"); err != nil {
		// Might already exist; try to find it.
	}
	cats, err := db.ListCatalogs()
	if err != nil || len(cats) == 0 {
		t.Fatalf("ListCatalogs: %v", err)
	}
	a, err := db.AddAgent(cats[0].ID, "test-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	return a.ID
}

func TestRunner_Run_SimpleTextResponse(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{
		textResponse("hello from agent"),
	}}
	runner, db := newRunnerDB(t, p)
	agentID := addTestAgent(t, db)

	result, err := runner.Run(context.Background(), agentID, "hi", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Response != "hello from agent" {
		t.Errorf("Response = %q, want %q", result.Response, "hello from agent")
	}
	if result.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", result.InputTokens)
	}
	if result.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", result.OutputTokens)
	}
}

func TestRunner_Run_AgentNotFound(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{textResponse("x")}}
	runner, _ := newRunnerDB(t, p)

	_, err := runner.Run(context.Background(), ksuid.New(), "hi", nil)
	if err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
}

func TestRunner_Run_ProviderError(t *testing.T) {
	p := &fakeProvider{err: fmt.Errorf("provider down")}
	runner, db := newRunnerDB(t, p)
	agentID := addTestAgent(t, db)

	_, err := runner.Run(context.Background(), agentID, "hi", nil)
	if err == nil {
		t.Fatal("expected error when provider fails, got nil")
	}
}

func TestRunner_Run_WithHistory(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{
		textResponse("follow-up response"),
	}}
	runner, db := newRunnerDB(t, p)
	agentID := addTestAgent(t, db)

	history := []llm.Message{
		{
			Role: llm.RoleUser,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: "first message"}},
		},
		{
			Role: llm.RoleAssistant,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: "first response"}},
		},
	}

	result, err := runner.Run(context.Background(), agentID, "second message", history)
	if err != nil {
		t.Fatalf("Run with history: %v", err)
	}
	if result.Response != "follow-up response" {
		t.Errorf("Response = %q, want %q", result.Response, "follow-up response")
	}
	// History should include original 2 + new user turn + assistant turn = 4 messages.
	if len(result.History) != 4 {
		t.Errorf("History len = %d, want 4", len(result.History))
	}
}

func TestRunner_Run_UsesAgentInstructions(t *testing.T) {
	var capturedSystemPrompt string
	captureProvider := &captureSystemPromptProvider{
		response: textResponse("ok"),
		onCall: func(req llm.CompletionRequest) {
			capturedSystemPrompt = req.SystemPrompt
		},
	}
	runner, db := newRunnerDB(t, captureProvider)
	agentID := addTestAgent(t, db)

	if err := db.SetAgentInstructions(agentID, "you are a test agent"); err != nil {
		t.Fatalf("SetAgentInstructions: %v", err)
	}

	_, err := runner.Run(context.Background(), agentID, "hi", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedSystemPrompt != "you are a test agent" {
		t.Errorf("SystemPrompt = %q, want %q", capturedSystemPrompt, "you are a test agent")
	}
}

// captureSystemPromptProvider captures the CompletionRequest for inspection.
type captureSystemPromptProvider struct {
	response *llm.CompletionResponse
	onCall   func(llm.CompletionRequest)
}

func (c *captureSystemPromptProvider) Complete(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if c.onCall != nil {
		c.onCall(req)
	}
	return c.response, nil
}

func TestRunner_RunWithOptions_SystemPromptSuffix(t *testing.T) {
	var capturedPrompt string
	captureProvider := &captureSystemPromptProvider{
		response: textResponse("ok"),
		onCall: func(req llm.CompletionRequest) {
			capturedPrompt = req.SystemPrompt
		},
	}
	runner, db := newRunnerDB(t, captureProvider)
	agentID := addTestAgent(t, db)

	if err := db.SetAgentInstructions(agentID, "base instructions"); err != nil {
		t.Fatalf("SetAgentInstructions: %v", err)
	}

	_, err := runner.RunWithOptions(context.Background(), agentID, "hi", nil, agent.RunOptions{
		SystemPromptSuffix: "extra instructions",
	})
	if err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}

	expected := "base instructions\n\nextra instructions"
	if capturedPrompt != expected {
		t.Errorf("SystemPrompt = %q, want %q", capturedPrompt, expected)
	}
}
