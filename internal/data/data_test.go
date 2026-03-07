package data_test

import (
	"testing"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
	"slate/internal/llm"
)

func newTestData(t *testing.T) *data.Data {
	t.Helper()
	db, err := data.New("test", t.TempDir())
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ---- Workspace tests ----

func TestAddWorkspace_Success(t *testing.T) {
	db := newTestData(t)
	if err := db.AddWorkspace("my-workspace"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
}

func TestAddWorkspace_DuplicateName(t *testing.T) {
	db := newTestData(t)
	if err := db.AddWorkspace("dup"); err != nil {
		t.Fatalf("first AddWorkspace: %v", err)
	}
	if err := db.AddWorkspace("dup"); err == nil {
		t.Fatal("expected error for duplicate workspace name, got nil")
	}
}

func TestRemoveWorkspace_Success(t *testing.T) {
	db := newTestData(t)
	if err := db.AddWorkspace("ws-to-remove"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	if err := db.RemoveWorkspace("ws-to-remove"); err != nil {
		t.Fatalf("RemoveWorkspace: %v", err)
	}
}

func TestRemoveWorkspace_NotFound(t *testing.T) {
	db := newTestData(t)
	if err := db.RemoveWorkspace("nonexistent"); err == nil {
		t.Fatal("expected error removing nonexistent workspace, got nil")
	}
}

// ---- Catalog tests ----

func TestAddCatalog_Success(t *testing.T) {
	db := newTestData(t)
	if err := db.AddCatalog("my-catalog"); err != nil {
		t.Fatalf("AddCatalog: %v", err)
	}
}

func TestAddCatalog_DuplicateName(t *testing.T) {
	db := newTestData(t)
	if err := db.AddCatalog("dup-cat"); err != nil {
		t.Fatalf("first AddCatalog: %v", err)
	}
	if err := db.AddCatalog("dup-cat"); err == nil {
		t.Fatal("expected error for duplicate catalog name, got nil")
	}
}

func TestRemoveCatalog_Success(t *testing.T) {
	db := newTestData(t)
	if err := db.AddCatalog("to-remove"); err != nil {
		t.Fatalf("AddCatalog: %v", err)
	}
	if err := db.RemoveCatalog("to-remove"); err != nil {
		t.Fatalf("RemoveCatalog: %v", err)
	}
}

func TestRemoveCatalog_NotFound(t *testing.T) {
	db := newTestData(t)
	if err := db.RemoveCatalog("nope"); err == nil {
		t.Fatal("expected error removing nonexistent catalog, got nil")
	}
}

func TestListCatalogs_Empty(t *testing.T) {
	db := newTestData(t)
	cats, err := db.ListCatalogs()
	if err != nil {
		t.Fatalf("ListCatalogs: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("expected 0 catalogs, got %d", len(cats))
	}
}

func TestListCatalogs_Multiple(t *testing.T) {
	db := newTestData(t)
	if err := db.AddCatalog("cat-a"); err != nil {
		t.Fatalf("AddCatalog a: %v", err)
	}
	if err := db.AddCatalog("cat-b"); err != nil {
		t.Fatalf("AddCatalog b: %v", err)
	}

	cats, err := db.ListCatalogs()
	if err != nil {
		t.Fatalf("ListCatalogs: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 catalogs, got %d", len(cats))
	}
}

// ---- Agent tests ----

// getCatalogID creates a catalog and returns its ID.
func getCatalogID(t *testing.T, db *data.Data, name string) ksuid.KSUID {
	t.Helper()
	if err := db.AddCatalog(name); err != nil {
		t.Fatalf("AddCatalog %q: %v", name, err)
	}
	cats, err := db.ListCatalogs()
	if err != nil {
		t.Fatalf("ListCatalogs: %v", err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("catalog %q not found after add", name)
	return ksuid.KSUID{}
}

func TestAddAgent_Success(t *testing.T) {
	db := newTestData(t)
	catID := getCatalogID(t, db, "cat1")

	agent, err := db.AddAgent(catID, "agent-1")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	if agent.Name != "agent-1" {
		t.Errorf("agent Name = %q, want %q", agent.Name, "agent-1")
	}
	if agent.ID == (ksuid.KSUID{}) {
		t.Error("agent ID should not be zero")
	}
}

func TestAddAgent_InvalidCatalog(t *testing.T) {
	db := newTestData(t)
	_, err := db.AddAgent(ksuid.New(), "agent-x")
	if err == nil {
		t.Fatal("expected error for unknown catalog, got nil")
	}
}

func TestFindAgent_Found(t *testing.T) {
	db := newTestData(t)
	catID := getCatalogID(t, db, "cat2")
	a, err := db.AddAgent(catID, "finder-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	found, _, err := db.FindAgent(a.ID)
	if err != nil {
		t.Fatalf("FindAgent: %v", err)
	}
	if found.ID != a.ID {
		t.Errorf("found agent ID mismatch: got %v, want %v", found.ID, a.ID)
	}
}

func TestFindAgent_NotFound(t *testing.T) {
	db := newTestData(t)
	_, _, err := db.FindAgent(ksuid.New())
	if err == nil {
		t.Fatal("expected error finding nonexistent agent, got nil")
	}
}

func TestSetAgentInstructions(t *testing.T) {
	db := newTestData(t)
	catID := getCatalogID(t, db, "cat3")
	a, err := db.AddAgent(catID, "instruct-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	if err := db.SetAgentInstructions(a.ID, "you are helpful"); err != nil {
		t.Fatalf("SetAgentInstructions: %v", err)
	}

	found, _, _ := db.FindAgent(a.ID)
	if found.Instructions != "you are helpful" {
		t.Errorf("Instructions = %q, want %q", found.Instructions, "you are helpful")
	}
}

func TestSetAgentModel(t *testing.T) {
	db := newTestData(t)
	catID := getCatalogID(t, db, "cat4")
	a, err := db.AddAgent(catID, "model-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	if err := db.SetAgentModel(a.ID, "claude-opus-4-6"); err != nil {
		t.Fatalf("SetAgentModel: %v", err)
	}

	found, _, _ := db.FindAgent(a.ID)
	if found.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", found.Model, "claude-opus-4-6")
	}
}

func TestAddAgentTool_And_Remove(t *testing.T) {
	db := newTestData(t)
	catID := getCatalogID(t, db, "cat5")
	a, err := db.AddAgent(catID, "tool-agent")
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	if err := db.AddAgentTool(a.ID, "http_fetch"); err != nil {
		t.Fatalf("AddAgentTool: %v", err)
	}

	// Duplicate add should fail.
	if err := db.AddAgentTool(a.ID, "http_fetch"); err == nil {
		t.Error("expected error adding duplicate tool, got nil")
	}

	if err := db.RemoveAgentTool(a.ID, "http_fetch"); err != nil {
		t.Fatalf("RemoveAgentTool: %v", err)
	}

	// Remove non-existent tool should fail.
	if err := db.RemoveAgentTool(a.ID, "missing_tool"); err == nil {
		t.Error("expected error removing nonexistent tool, got nil")
	}
}

// ---- Job tests ----

func TestCreateJob(t *testing.T) {
	db := newTestData(t)

	job, err := db.CreateJob("pipeline_run", ksuid.New(), ksuid.New(), "hello input")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.Input != "hello input" {
		t.Errorf("Input = %q, want %q", job.Input, "hello input")
	}
	if job.Status != data.JobPending {
		t.Errorf("Status = %q, want %q", job.Status, data.JobPending)
	}
	if job.ID == (ksuid.KSUID{}) {
		t.Error("job ID should not be zero")
	}
}

func TestGetJob_Found(t *testing.T) {
	db := newTestData(t)
	job, _ := db.CreateJob("test", ksuid.New(), ksuid.New(), "in")
	got, err := db.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("job ID mismatch")
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.GetJob(ksuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent job, got nil")
	}
}

func TestUpdateJob_Running(t *testing.T) {
	db := newTestData(t)
	job, _ := db.CreateJob("test", ksuid.New(), ksuid.New(), "in")

	if err := db.UpdateJob(job.ID, data.JobRunning, "", ""); err != nil {
		t.Fatalf("UpdateJob running: %v", err)
	}
	got, _ := db.GetJob(job.ID)
	if got.Status != data.JobRunning {
		t.Errorf("Status = %q, want %q", got.Status, data.JobRunning)
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt should be set when job is running")
	}
}

func TestUpdateJob_Completed(t *testing.T) {
	db := newTestData(t)
	job, _ := db.CreateJob("test", ksuid.New(), ksuid.New(), "in")

	if err := db.UpdateJob(job.ID, data.JobCompleted, "done!", ""); err != nil {
		t.Fatalf("UpdateJob completed: %v", err)
	}
	got, _ := db.GetJob(job.ID)
	if got.Status != data.JobCompleted {
		t.Errorf("Status = %q, want %q", got.Status, data.JobCompleted)
	}
	if got.Result != "done!" {
		t.Errorf("Result = %q, want %q", got.Result, "done!")
	}
	if got.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set when job is completed")
	}
}

func TestUpdateJob_Failed(t *testing.T) {
	db := newTestData(t)
	job, _ := db.CreateJob("test", ksuid.New(), ksuid.New(), "in")

	if err := db.UpdateJob(job.ID, data.JobFailed, "", "something went wrong"); err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}
	got, _ := db.GetJob(job.ID)
	if got.Status != data.JobFailed {
		t.Errorf("Status = %q, want %q", got.Status, data.JobFailed)
	}
	if got.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", got.Error, "something went wrong")
	}
}

func TestCancelJob(t *testing.T) {
	db := newTestData(t)
	job, _ := db.CreateJob("test", ksuid.New(), ksuid.New(), "in")

	cancelled := false
	_ = db.SetJobCancel(job.ID, func() { cancelled = true })

	if err := db.CancelJob(job.ID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if !cancelled {
		t.Error("cancel function was not called")
	}
}

func TestListJobs_AllWorkspaces(t *testing.T) {
	db := newTestData(t)
	wsID1 := ksuid.New()
	wsID2 := ksuid.New()
	_, _ = db.CreateJob("t1", wsID1, ksuid.New(), "a")
	_, _ = db.CreateJob("t2", wsID2, ksuid.New(), "b")
	_, _ = db.CreateJob("t3", wsID1, ksuid.New(), "c")

	// Zero KSUID = list all jobs.
	jobs, err := db.ListJobs(ksuid.KSUID{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestListJobs_FilteredByWorkspace(t *testing.T) {
	db := newTestData(t)
	wsID1 := ksuid.New()
	wsID2 := ksuid.New()
	_, _ = db.CreateJob("t1", wsID1, ksuid.New(), "a")
	_, _ = db.CreateJob("t2", wsID2, ksuid.New(), "b")
	_, _ = db.CreateJob("t3", wsID1, ksuid.New(), "c")

	jobs, err := db.ListJobs(wsID1)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs for wsID1, got %d", len(jobs))
	}
}

// ---- Thread tests ----

// setupWorkspace creates a workspace and returns its ID.
func setupWorkspace(t *testing.T, db *data.Data, name string) ksuid.KSUID {
	t.Helper()
	if err := db.AddWorkspace(name); err != nil {
		t.Fatalf("AddWorkspace %q: %v", name, err)
	}
	for id, w := range db.Workspaces {
		if w.Name == name {
			return id
		}
	}
	t.Fatalf("workspace %q not found after add", name)
	return ksuid.KSUID{}
}

func TestNewThread_Success(t *testing.T) {
	db := newTestData(t)
	wsID := setupWorkspace(t, db, "th-ws")

	thread, err := db.NewThread(wsID, "my-thread")
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}
	if thread.Name != "my-thread" {
		t.Errorf("thread Name = %q, want %q", thread.Name, "my-thread")
	}
	if thread.WorkspaceID != wsID {
		t.Error("thread WorkspaceID mismatch")
	}
}

func TestGetThread_Found(t *testing.T) {
	db := newTestData(t)
	wsID := setupWorkspace(t, db, "th-ws2")

	thread, err := db.NewThread(wsID, "t1")
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}

	got, err := db.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if got.ID != thread.ID {
		t.Error("thread ID mismatch")
	}
}

func TestGetThread_NotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.GetThread(ksuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent thread, got nil")
	}
}

func TestDeleteThread(t *testing.T) {
	db := newTestData(t)
	wsID := setupWorkspace(t, db, "th-ws3")

	thread, err := db.NewThread(wsID, "to-delete")
	if err != nil {
		t.Fatalf("NewThread: %v", err)
	}
	if err := db.DeleteThread(thread.ID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if _, err := db.GetThread(thread.ID); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestListThreads_ByWorkspace(t *testing.T) {
	db := newTestData(t)
	wsID := setupWorkspace(t, db, "th-ws4")

	_, _ = db.NewThread(wsID, "t1")
	_, _ = db.NewThread(wsID, "t2")

	threads, err := db.ListThreads(wsID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Errorf("expected 2 threads, got %d", len(threads))
	}
}

// ---- AgentThread tests ----

// getAgentID creates a catalog+agent and returns the agent's ID.
func getAgentID(t *testing.T, db *data.Data, suffix string) ksuid.KSUID {
	t.Helper()
	catName := "at-cat-" + suffix
	catID := getCatalogID(t, db, catName)
	a, err := db.AddAgent(catID, "at-agent-"+suffix)
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	return a.ID
}

func TestNewAgentThread_Success(t *testing.T) {
	db := newTestData(t)
	agentID := getAgentID(t, db, "1")

	thread, err := db.NewAgentThread(agentID, "my-agent-thread")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}
	if thread.Name != "my-agent-thread" {
		t.Errorf("Name = %q, want %q", thread.Name, "my-agent-thread")
	}
	if thread.AgentID != agentID {
		t.Error("AgentID mismatch")
	}
	if thread.ID == (ksuid.KSUID{}) {
		t.Error("ID should not be zero")
	}
}

func TestNewAgentThread_AgentNotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.NewAgentThread(ksuid.New(), "nope")
	if err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
}

func TestGetAgentThread_Found(t *testing.T) {
	db := newTestData(t)
	agentID := getAgentID(t, db, "2")

	thread, err := db.NewAgentThread(agentID, "get-me")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}

	got, err := db.GetAgentThread(thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread: %v", err)
	}
	if got.ID != thread.ID {
		t.Error("ID mismatch")
	}
}

func TestGetAgentThread_NotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.GetAgentThread(ksuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent agent thread, got nil")
	}
}

func TestDeleteAgentThread(t *testing.T) {
	db := newTestData(t)
	agentID := getAgentID(t, db, "3")

	thread, err := db.NewAgentThread(agentID, "delete-me")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}
	if err := db.DeleteAgentThread(thread.ID); err != nil {
		t.Fatalf("DeleteAgentThread: %v", err)
	}
	if _, err := db.GetAgentThread(thread.ID); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestListAgentThreads_ByAgent(t *testing.T) {
	db := newTestData(t)
	agentID := getAgentID(t, db, "4")

	_, _ = db.NewAgentThread(agentID, "t1")
	_, _ = db.NewAgentThread(agentID, "t2")

	threads, err := db.ListAgentThreads(agentID)
	if err != nil {
		t.Fatalf("ListAgentThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Errorf("expected 2 threads, got %d", len(threads))
	}
}

func TestListAgentThreads_AgentNotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.ListAgentThreads(ksuid.New())
	if err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
}

func TestListAgentThreads_DoesNotCrossAgents(t *testing.T) {
	db := newTestData(t)
	agentID1 := getAgentID(t, db, "5a")
	agentID2 := getAgentID(t, db, "5b")

	_, _ = db.NewAgentThread(agentID1, "agent1-thread")
	_, _ = db.NewAgentThread(agentID2, "agent2-thread")

	threads, err := db.ListAgentThreads(agentID1)
	if err != nil {
		t.Fatalf("ListAgentThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Errorf("expected 1 thread for agent1, got %d", len(threads))
	}
}

func TestAppendAgentMessage(t *testing.T) {
	db := newTestData(t)
	agentID := getAgentID(t, db, "6")

	thread, err := db.NewAgentThread(agentID, "msg-thread")
	if err != nil {
		t.Fatalf("NewAgentThread: %v", err)
	}

	msg := llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.Content{{Type: llm.ContentTypeText, Text: "hello"}},
	}
	if err := db.AppendAgentMessage(thread.ID, msg); err != nil {
		t.Fatalf("AppendAgentMessage: %v", err)
	}

	got, err := db.GetAgentThread(thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(got.Messages))
	}
}

// ---- Pipeline tests ----

func TestNewPipeline_Success(t *testing.T) {
	db := newTestData(t)
	if err := db.AddWorkspace("pipe-ws"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	var wsID ksuid.KSUID
	for id, w := range db.Workspaces {
		if w.Name == "pipe-ws" {
			wsID = id
			break
		}
	}

	p, err := db.NewPipeline(wsID, "my-pipeline")
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	if p.Name != "my-pipeline" {
		t.Errorf("pipeline Name = %q, want %q", p.Name, "my-pipeline")
	}
}

func TestGetPipeline_NotFound(t *testing.T) {
	db := newTestData(t)
	_, err := db.GetPipeline(ksuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline, got nil")
	}
}

func TestSetWorkspaceCatalog(t *testing.T) {
	db := newTestData(t)
	if err := db.AddWorkspace("ws-cat-link"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	var wsID ksuid.KSUID
	for id, w := range db.Workspaces {
		if w.Name == "ws-cat-link" {
			wsID = id
			break
		}
	}
	catID := getCatalogID(t, db, "linked-cat")

	if err := db.SetWorkspaceCatalog(wsID, catID); err != nil {
		t.Fatalf("SetWorkspaceCatalog: %v", err)
	}

	ws, err := db.GetWorkspace(wsID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if ws.CatalogID != catID {
		t.Error("workspace CatalogID not updated")
	}
}
