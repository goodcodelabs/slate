package agent_test

import (
	"context"
	"fmt"
	"strings"
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

func TestRunner_RunAgentThread_Success(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{
		textResponse("agent thread response"),
	}}
	runner, db := newRunnerDB(t, p)
	agentID := addTestAgent(t, db)

	thread, err := db.NewAgentThread(agentID, "my-thread")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}

	result, err := runner.RunAgentThread(context.Background(), thread.ID, "hello")
	if err != nil {
		t.Fatalf("RunAgentThread: %v", err)
	}
	if result.Response != "agent thread response" {
		t.Errorf("Response = %q, want %q", result.Response, "agent thread response")
	}

	// Messages should be persisted to the thread.
	updated, err := db.GetAgentThread(thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread after run: %v", err)
	}
	if len(updated.Messages) < 2 {
		t.Errorf("expected at least 2 messages persisted, got %d", len(updated.Messages))
	}
}

func TestRunner_RunAgentThread_WithHistory(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{
		textResponse("first"),
		textResponse("second"),
	}}
	runner, db := newRunnerDB(t, p)
	agentID := addTestAgent(t, db)

	thread, err := db.NewAgentThread(agentID, "history-thread")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}

	// First turn.
	if _, err := runner.RunAgentThread(context.Background(), thread.ID, "turn 1"); err != nil {
		t.Fatalf("first RunAgentThread: %v", err)
	}

	// Second turn — should see the prior messages in history.
	result, err := runner.RunAgentThread(context.Background(), thread.ID, "turn 2")
	if err != nil {
		t.Fatalf("second RunAgentThread: %v", err)
	}
	if result.Response != "second" {
		t.Errorf("Response = %q, want %q", result.Response, "second")
	}

	updated, err := db.GetAgentThread(thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread: %v", err)
	}
	// 2 turns × (user + assistant) = 4 messages.
	if len(updated.Messages) != 4 {
		t.Errorf("expected 4 messages after 2 turns, got %d", len(updated.Messages))
	}
}

func TestRunner_RunAgentThread_AgentNotFound(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{textResponse("x")}}
	runner, db := newRunnerDB(t, p)

	// Create agent thread then delete the underlying agent by pointing to an unknown ID.
	// Easiest: just create a thread with a fake agentID directly in the map.
	fakeID := ksuid.New()
	db.Threads[fakeID] = &data.Thread{
		ID:      fakeID,
		AgentID: ksuid.New(), // nonexistent agent
		Name:    "orphan",
		State:   data.ThreadActive,
	}

	_, err := runner.RunAgentThread(context.Background(), fakeID, "hi")
	if err == nil {
		t.Fatal("expected error when agent not found, got nil")
	}
}

func TestRunner_RunAgentThread_ThreadNotFound(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{textResponse("x")}}
	runner, _ := newRunnerDB(t, p)

	_, err := runner.RunAgentThread(context.Background(), ksuid.New(), "hi")
	if err == nil {
		t.Fatal("expected error for nonexistent thread, got nil")
	}
}

func TestRunner_RunThread_NoRouter(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{textResponse("x")}}
	runner, db := newRunnerDB(t, p)

	// Create a workspace without a router agent.
	if err := db.AddWorkspace("no-router-ws"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	var wsID ksuid.KSUID
	for id, w := range db.Workspaces {
		if w.Name == "no-router-ws" {
			wsID = id
			break
		}
	}

	thread, err := db.NewThread(wsID, "test-thread")
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}

	_, err = runner.RunThread(context.Background(), thread.ID, "hello")
	if err == nil {
		t.Fatal("expected error when workspace has no router, got nil")
	}
	if !strings.Contains(err.Error(), "router") {
		t.Errorf("error %q should mention router", err.Error())
	}
}

func TestRunner_RunThread_RoutesViaWorkspaceRouter(t *testing.T) {
	p := &fakeProvider{responses: []*llm.CompletionResponse{
		textResponse("router response"),
	}}
	runner, db := newRunnerDB(t, p)

	// Set up workspace with catalog and router agent.
	if err := db.AddWorkspace("router-ws"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	var wsID ksuid.KSUID
	for id, w := range db.Workspaces {
		if w.Name == "router-ws" {
			wsID = id
			break
		}
	}

	routerAgentID := addTestAgent(t, db)

	if err := db.SetWorkspaceRouter(wsID, routerAgentID); err != nil {
		t.Fatalf("SetWorkspaceRouter: %v", err)
	}

	thread, err := db.NewThread(wsID, "router-thread")
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}

	result, err := runner.RunThread(context.Background(), thread.ID, "hello")
	if err != nil {
		t.Fatalf("RunThread: %v", err)
	}
	if result.Response != "router response" {
		t.Errorf("Response = %q, want %q", result.Response, "router response")
	}

	// Messages should be persisted to the thread.
	updated, err := db.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread after RunThread: %v", err)
	}
	// Should have at least the user message and assistant response.
	if len(updated.Messages) < 2 {
		t.Errorf("expected at least 2 messages persisted, got %d", len(updated.Messages))
	}
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
