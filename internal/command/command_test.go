package command_test

import (
	"encoding/json"
	"testing"

	"slate/internal/command"
	"slate/internal/data"
	"slate/internal/scheduler"
)

func commandsWithSched(store *data.Data, sched *scheduler.Scheduler) map[string]command.ProtocolCommand {
	return command.InitCommands(store, nil, sched, nil)
}

func newTestStore(t *testing.T) *data.Data {
	t.Helper()
	db, err := data.New("test", t.TempDir())
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// commands builds a command map for testing — runner, sched, metrics can be nil
// for commands that only use the store.
func commands(store *data.Data) map[string]command.ProtocolCommand {
	return command.InitCommands(store, nil, nil, nil)
}

// exec runs a named command with the given params (marshaled to JSON).
func exec(t *testing.T, cmds map[string]command.ProtocolCommand, name string, params interface{}) (*command.Response, error) {
	t.Helper()
	cmd, ok := cmds[name]
	if !ok {
		t.Fatalf("command %q not found", name)
	}
	var raw json.RawMessage
	if params == nil {
		raw = json.RawMessage("{}")
	} else {
		var err error
		raw, err = json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
	}
	return cmd.Execute(command.Context{}, raw)
}

// ---- Health tests ----

func TestHealthCommand_ReturnsOk(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdHealth, nil)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if resp == nil || resp.Message != "ok" {
		t.Errorf("health response = %v, want 'ok'", resp)
	}
}

// ---- Workspace command tests ----

func TestAddWorkspaceCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdAddWorkspace, map[string]string{"name": "my-workspace"})
	if err != nil {
		t.Fatalf("add_workspace: %v", err)
	}
	if resp.Message != "ok" {
		t.Errorf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestAddWorkspaceCommand_Duplicate(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, _ = exec(t, cmds, command.CmdAddWorkspace, map[string]string{"name": "dup-ws"})
	_, err := exec(t, cmds, command.CmdAddWorkspace, map[string]string{"name": "dup-ws"})
	if err == nil {
		t.Fatal("expected error for duplicate workspace name, got nil")
	}
}

func TestRemoveWorkspaceCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, _ = exec(t, cmds, command.CmdAddWorkspace, map[string]string{"name": "to-remove"})
	resp, err := exec(t, cmds, command.CmdDelWorkspace, map[string]string{"name": "to-remove"})
	if err != nil {
		t.Fatalf("del_workspace: %v", err)
	}
	if resp.Message != "ok" {
		t.Errorf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestRemoveWorkspaceCommand_NotFound(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdDelWorkspace, map[string]string{"name": "nonexistent"})
	if err == nil {
		t.Fatal("expected error removing nonexistent workspace, got nil")
	}
}

// ---- Catalog command tests ----

func TestAddCatalogCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdAddCatalog, map[string]string{"name": "my-catalog"})
	if err != nil {
		t.Fatalf("add_catalog: %v", err)
	}
	if resp.Message != "ok" {
		t.Errorf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestAddCatalogCommand_Duplicate(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, _ = exec(t, cmds, command.CmdAddCatalog, map[string]string{"name": "dup-cat"})
	_, err := exec(t, cmds, command.CmdAddCatalog, map[string]string{"name": "dup-cat"})
	if err == nil {
		t.Fatal("expected error for duplicate catalog, got nil")
	}
}

func TestRemoveCatalogCommand_NotFound(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdDelCatalog, map[string]string{"name": "missing"})
	if err == nil {
		t.Fatal("expected error for missing catalog, got nil")
	}
}

func TestListCatalogsCommand_Empty(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdListCatalogs, nil)
	if err != nil {
		t.Fatalf("ls_catalogs: %v", err)
	}

	var out struct {
		Catalogs []interface{} `json:"catalogs"`
	}
	if err := json.Unmarshal([]byte(resp.Message), &out); err != nil {
		t.Fatalf("parsing response %q: %v", resp.Message, err)
	}
	if len(out.Catalogs) != 0 {
		t.Errorf("expected 0 catalogs, got %d", len(out.Catalogs))
	}
}

func TestListCatalogsCommand_WithEntries(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, _ = exec(t, cmds, command.CmdAddCatalog, map[string]string{"name": "cat-a"})
	_, _ = exec(t, cmds, command.CmdAddCatalog, map[string]string{"name": "cat-b"})

	resp, err := exec(t, cmds, command.CmdListCatalogs, nil)
	if err != nil {
		t.Fatalf("ls_catalogs: %v", err)
	}

	var out struct {
		Catalogs []interface{} `json:"catalogs"`
	}
	json.Unmarshal([]byte(resp.Message), &out)
	if len(out.Catalogs) != 2 {
		t.Errorf("expected 2 catalogs, got %d", len(out.Catalogs))
	}
}

// ---- AgentThread command tests ----

// setupCatalogAndAgent creates a catalog+agent in the store and returns the agent ID string.
func setupCatalogAndAgent(t *testing.T, store *data.Data) string {
	t.Helper()
	if err := store.AddCatalog("cmd-test-cat"); err != nil {
		t.Fatalf("AddCatalog: %v", err)
	}
	cats, err := store.ListCatalogs()
	if err != nil || len(cats) == 0 {
		t.Fatalf("ListCatalogs: %v", err)
	}
	a, err := store.AddAgent(cats[0].ID, "cmd-test-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	return a.ID.String()
}

func TestNewAgentThreadCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "my-thread"})
	if err != nil {
		t.Fatalf("new_agent_thread: %v", err)
	}

	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(resp.Message), &out); err != nil {
		t.Fatalf("parsing response: %v", err)
	}
	if out.ID == "" {
		t.Error("expected non-empty thread ID")
	}
	if out.Name != "my-thread" {
		t.Errorf("Name = %q, want %q", out.Name, "my-thread")
	}
}

func TestNewAgentThreadCommand_AutoName(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID})
	if err != nil {
		t.Fatalf("new_agent_thread: %v", err)
	}

	var out struct {
		Name string `json:"name"`
	}
	json.Unmarshal([]byte(resp.Message), &out)
	if out.Name == "" {
		t.Error("expected auto-generated name, got empty string")
	}
}

func TestNewAgentThreadCommand_MissingParams(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing agent_id, got nil")
	}
}

func TestNewAgentThreadCommand_InvalidAgentKSUID(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": "not-a-valid-ksuid"})
	if err == nil {
		t.Fatal("expected error for invalid KSUID, got nil")
	}
}

func TestNewAgentThreadCommand_AgentNotFound(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": "2cDKvMGSMqCjFpuSkNdRaR7EiSa"})
	if err == nil {
		t.Fatal("expected error for nonexistent agent, got nil")
	}
}

func TestListAgentThreadsCommand_Empty(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, err := exec(t, cmds, command.CmdListAgentThreads, map[string]string{"agent_id": agentID})
	if err != nil {
		t.Fatalf("ls_agent_threads: %v", err)
	}

	var out struct {
		Threads []interface{} `json:"threads"`
	}
	json.Unmarshal([]byte(resp.Message), &out)
	if len(out.Threads) != 0 {
		t.Errorf("expected 0 threads, got %d", len(out.Threads))
	}
}

func TestListAgentThreadsCommand_WithEntries(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	_, _ = exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "t1"})
	_, _ = exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "t2"})

	resp, err := exec(t, cmds, command.CmdListAgentThreads, map[string]string{"agent_id": agentID})
	if err != nil {
		t.Fatalf("ls_agent_threads: %v", err)
	}

	var out struct {
		Threads []interface{} `json:"threads"`
	}
	json.Unmarshal([]byte(resp.Message), &out)
	if len(out.Threads) != 2 {
		t.Errorf("expected 2 threads, got %d", len(out.Threads))
	}
}

func TestAgentThreadHistoryCommand_Empty(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, _ := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "hist-thread"})
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(resp.Message), &created)

	histResp, err := exec(t, cmds, command.CmdAgentThreadHistory, map[string]string{"thread_id": created.ID})
	if err != nil {
		t.Fatalf("agent_thread_history: %v", err)
	}

	var out struct {
		Messages []interface{} `json:"messages"`
	}
	json.Unmarshal([]byte(histResp.Message), &out)
	if len(out.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(out.Messages))
	}
}

func TestAgentChatCommand_MissingMessage(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, _ := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "t"})
	var created struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(resp.Message), &created)

	_, err := exec(t, cmds, command.CmdAgentChat, map[string]string{"thread_id": created.ID})
	if err == nil {
		t.Fatal("expected error for missing message param, got nil")
	}
}

// ---- SetWorkspaceCatalog command tests ----

func TestSetWorkspaceCatalogCommand_InvalidKSUID(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceCatalog, map[string]string{
		"workspace_id": "not-a-ksuid",
		"catalog_id":   "also-not",
	})
	if err == nil {
		t.Fatal("expected error for invalid KSUID, got nil")
	}
}

// ---- SetWorkspaceRouter command tests ----

func TestSetWorkspaceRouterCommand_InvalidKSUID(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceRouter, map[string]string{
		"workspace_id": "bad-id",
		"agent_id":     "bad-id2",
	})
	if err == nil {
		t.Fatal("expected error for invalid KSUIDs, got nil")
	}
}

// ---- Async command tests ----

func newSchedForTest(t *testing.T) *scheduler.Scheduler {
	t.Helper()
	return scheduler.NewScheduler(1)
}

func TestRunAgentCommand_ReturnsJobID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)
	agentID := setupCatalogAndAgent(t, store)

	resp, err := exec(t, cmds, command.CmdRunAgent, map[string]string{"agent_id": agentID, "input": "hello"})
	if err != nil {
		t.Fatalf("run_agent: %v", err)
	}

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(resp.Message), &out); err != nil {
		t.Fatalf("parsing response %q: %v", resp.Message, err)
	}
	if out.JobID == "" {
		t.Error("expected non-empty job_id")
	}
}

func TestRunAgentCommand_InvalidAgentID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)

	_, err := exec(t, cmds, command.CmdRunAgent, map[string]string{"agent_id": "not-a-ksuid", "input": "hello"})
	if err == nil {
		t.Fatal("expected error for invalid agent_id, got nil")
	}
}

func TestRunAgentCommand_AgentNotFound(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)

	_, err := exec(t, cmds, command.CmdRunAgent, map[string]string{"agent_id": "2cDKvMGSMqCjFpuSkNdRaR7EiSa", "input": "hello"})
	if err == nil {
		t.Fatal("expected error for nonexistent agent, got nil")
	}
}

func TestChatCommand_ReturnsJobID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)

	if err := store.AddWorkspace("chat-ws"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	var wsID string
	for id := range store.Workspaces {
		wsID = id.String()
		break
	}

	tResp, err := exec(t, cmds, command.CmdNewThread, map[string]string{"workspace_id": wsID, "name": "chat-thread"})
	if err != nil {
		t.Fatalf("new_thread: %v", err)
	}
	var thread struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(tResp.Message), &thread)

	resp, err := exec(t, cmds, command.CmdChat, map[string]string{"thread_id": thread.ID, "message": "hello"})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(resp.Message), &out); err != nil {
		t.Fatalf("parsing response %q: %v", resp.Message, err)
	}
	if out.JobID == "" {
		t.Error("expected non-empty job_id")
	}
}

func TestChatCommand_InvalidThreadID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)

	_, err := exec(t, cmds, command.CmdChat, map[string]string{"thread_id": "not-a-ksuid", "message": "hello"})
	if err == nil {
		t.Fatal("expected error for invalid thread_id, got nil")
	}
}

func TestAgentChatCommand_ReturnsJobID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)
	agentID := setupCatalogAndAgent(t, store)

	tResp, err := exec(t, cmds, command.CmdNewAgentThread, map[string]string{"agent_id": agentID, "name": "ac-thread"})
	if err != nil {
		t.Fatalf("new_agent_thread: %v", err)
	}
	var thread struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(tResp.Message), &thread)

	resp, err := exec(t, cmds, command.CmdAgentChat, map[string]string{"thread_id": thread.ID, "message": "hello"})
	if err != nil {
		t.Fatalf("agent_chat: %v", err)
	}

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(resp.Message), &out); err != nil {
		t.Fatalf("parsing response %q: %v", resp.Message, err)
	}
	if out.JobID == "" {
		t.Error("expected non-empty job_id")
	}
}

func TestAgentChatCommand_InvalidThreadID(t *testing.T) {
	store := newTestStore(t)
	sched := newSchedForTest(t)
	cmds := commandsWithSched(store, sched)

	_, err := exec(t, cmds, command.CmdAgentChat, map[string]string{"thread_id": "not-a-ksuid", "message": "hello"})
	if err == nil {
		t.Fatal("expected error for invalid thread_id, got nil")
	}
}

// ---- del_agent command tests ----

func TestDelAgentCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)
	agentID := setupCatalogAndAgent(t, store)

	resp, err := exec(t, cmds, command.CmdDelAgent, map[string]string{"agent_id": agentID})
	if err != nil {
		t.Fatalf("del_agent: %v", err)
	}
	if resp.Message != "ok" {
		t.Errorf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestDelAgentCommand_NotFound(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdDelAgent, map[string]string{"agent_id": "2cDKvMGSMqCjFpuSkNdRaR7EiSa"})
	if err == nil {
		t.Fatal("expected error for nonexistent agent, got nil")
	}
}
