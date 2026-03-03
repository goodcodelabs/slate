package command_test

import (
	"encoding/json"
	"testing"

	"slate/internal/command"
	"slate/internal/data"
)

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

func exec(t *testing.T, cmds map[string]command.ProtocolCommand, name string, params ...string) (*command.Response, error) {
	t.Helper()
	cmd, ok := cmds[name]
	if !ok {
		t.Fatalf("command %q not found", name)
	}
	return cmd.Execute(command.Context{}, params)
}

// ---- Health tests ----

func TestHealthCommand_ReturnsOk(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdHealth)
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

	resp, err := exec(t, cmds, command.CmdAddWorkspace, "my-workspace")
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

	_, _ = exec(t, cmds, command.CmdAddWorkspace, "dup-ws")
	_, err := exec(t, cmds, command.CmdAddWorkspace, "dup-ws")
	if err == nil {
		t.Fatal("expected error for duplicate workspace name, got nil")
	}
}

func TestRemoveWorkspaceCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, _ = exec(t, cmds, command.CmdAddWorkspace, "to-remove")
	resp, err := exec(t, cmds, command.CmdDelWorkspace, "to-remove")
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

	_, err := exec(t, cmds, command.CmdDelWorkspace, "nonexistent")
	if err == nil {
		t.Fatal("expected error removing nonexistent workspace, got nil")
	}
}

// ---- Catalog command tests ----

func TestAddCatalogCommand_Success(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdAddCatalog, "my-catalog")
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

	_, _ = exec(t, cmds, command.CmdAddCatalog, "dup-cat")
	_, err := exec(t, cmds, command.CmdAddCatalog, "dup-cat")
	if err == nil {
		t.Fatal("expected error for duplicate catalog, got nil")
	}
}

func TestRemoveCatalogCommand_NotFound(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdDelCatalog, "missing")
	if err == nil {
		t.Fatal("expected error for missing catalog, got nil")
	}
}

func TestListCatalogsCommand_Empty(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	resp, err := exec(t, cmds, command.CmdListCatalogs)
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

	_, _ = exec(t, cmds, command.CmdAddCatalog, "cat-a")
	_, _ = exec(t, cmds, command.CmdAddCatalog, "cat-b")

	resp, err := exec(t, cmds, command.CmdListCatalogs)
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

// ---- SetWorkspaceCatalog command tests ----

func TestSetWorkspaceCatalogCommand_TooFewParams(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceCatalog, "only-one-param")
	if err == nil {
		t.Fatal("expected error for insufficient params, got nil")
	}
}

func TestSetWorkspaceCatalogCommand_InvalidKSUID(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceCatalog, "not-a-ksuid", "also-not")
	if err == nil {
		t.Fatal("expected error for invalid KSUID, got nil")
	}
}

// ---- SetWorkspaceRouter command tests ----

func TestSetWorkspaceRouterCommand_TooFewParams(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceRouter, "one-param")
	if err == nil {
		t.Fatal("expected error for insufficient params, got nil")
	}
}

func TestSetWorkspaceRouterCommand_InvalidKSUID(t *testing.T) {
	store := newTestStore(t)
	cmds := commands(store)

	_, err := exec(t, cmds, command.CmdSetWorkspaceRouter, "bad-id", "bad-id2")
	if err == nil {
		t.Fatal("expected error for invalid KSUIDs, got nil")
	}
}
