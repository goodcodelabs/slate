# Plan: Workspace-Centric Threads

## Problem

Threads currently require an `agent_id` at creation time and are permanently bound to a single agent. This makes them independent of the workspace's router, inconsistent with `workspace_chat`, and inconvenient — callers must know which agent to target before starting a conversation.

## Goal

Threads should belong to a workspace, not an agent. Creating a thread requires only a `workspace_id`. Chatting on a thread routes through the workspace's router agent (same logic as `workspace_chat`) but with the full accumulated thread history passed as conversation context.

---

## Changes

### 1. `internal/data/types.go`

Remove `AgentID` from `Thread`:

```go
type Thread struct {
    ID          ksuid.KSUID `msgpack:"id"`
    WorkspaceID ksuid.KSUID `msgpack:"workspace_id"`
    // AgentID removed
    Name        string      `msgpack:"name"`
    State       ThreadState `msgpack:"state"`
    CreatedAt   time.Time   `msgpack:"created_at"`
    UpdatedAt   time.Time   `msgpack:"updated_at"`
    Messages    []llm.Message `msgpack:"-"`
}
```

**Backward compatibility**: existing msgpack snapshots contain `agent_id`. When loaded into the new struct, msgpack ignores unknown fields — no migration required.

---

### 2. `internal/data/data.go`

Drop the `agentID` parameter from `NewThread` and remove the agent existence check:

```go
// Before
func (db *Data) NewThread(workspaceID, agentID ksuid.KSUID, name string) (*Thread, error)

// After
func (db *Data) NewThread(workspaceID ksuid.KSUID, name string) (*Thread, error)
```

The body removes:
- `if _, _, err := db.FindAgent(agentID); err != nil { ... }` check
- `AgentID: agentID` in the struct literal

No other data layer methods change.

---

### 3. `internal/agent/runner.go`

`RunThread` currently delegates to `r.Run(ctx, thread.AgentID, input, thread.Messages)`. Replace that with the same workspace-router logic used by `RunWorkspaceChat`, but passing the thread's accumulated history:

```go
func (r *Runner) RunThread(ctx context.Context, threadID ksuid.KSUID, input string) (*RunResult, error) {
    thread, err := r.store.GetThread(threadID)
    // ...

    workspace, err := r.store.GetWorkspace(thread.WorkspaceID)
    if err != nil {
        return nil, fmt.Errorf("loading workspace: %w", err)
    }
    if workspace.Config == nil || workspace.Config.RouterAgentID == (ksuid.KSUID{}) {
        return nil, fmt.Errorf("workspace has no router agent configured")
    }

    // Build catalog listing suffix (same as RunWorkspaceChat).
    suffix := buildCatalogListing(r, workspace)

    historyLen := len(thread.Messages)
    result, err := r.RunWithOptions(ctx, workspace.Config.RouterAgentID, input, thread.Messages, RunOptions{
        SystemPromptSuffix: suffix,
    })
    // ... persist new messages, emit events (unchanged)
}
```

Extract the catalog-listing builder from `RunWorkspaceChat` into a private helper `buildCatalogListing(r *Runner, ws *data.Workspace) string` so both methods share it without duplication.

Events emitted by `RunThread` should use `workspace.Config.RouterAgentID.String()` for the `AgentID` field instead of `thread.AgentID`.

---

### 4. `internal/command/thread.go`

**`NewThreadCommand`**: remove the `agent_id` positional parameter.

```
Before: new_thread <workspace_id> <agent_id> [name...]
After:  new_thread <workspace_id> [name...]
```

- Parse only `params[0]` as `workspace_id` (KSUID).
- Everything after is the optional name (joined with spaces). Auto-generate if absent.
- Call `db.NewThread(wsID, name)`.
- Response stays the same: `{"id":"...","name":"..."}`.

**`ListThreadsCommand`**: the response currently includes `agent_id` in each entry. Remove it from the JSON output since `Thread` no longer carries it.

**`ChatCommand`**: no changes — it calls `runner.RunThread(ctx, threadID, input)` which now handles routing internally.

---

### 5. `pkg/client/client.go`

Drop `agentID` from `NewThread`:

```go
// Before
func (c *Client) NewThread(workspaceID, agentID string, name string) (ThreadInfo, error)

// After
func (c *Client) NewThread(workspaceID string, name string) (ThreadInfo, error)
```

Wire line changes from `"new_thread", workspaceID, agentID` → `"new_thread", workspaceID`.

---

### 6. `pkg/client/types.go`

Remove `AgentID` from `ThreadInfo`:

```go
type ThreadInfo struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    // AgentID removed
    State     string `json:"state"`
    Messages  int    `json:"messages"`
    CreatedAt string `json:"created_at"`
    UpdatedAt string `json:"updated_at"`
}
```

---

## Tests to update

### `internal/data/data_test.go`

- `setupWsAndAgent` helper is no longer needed for thread tests. Replace `setupWsAndAgent` with a simpler `setupWorkspace` that just creates a workspace and returns its ID.
- `TestNewThread_Success`, `TestGetThread_Found`, `TestDeleteThread`, `TestListThreads_ByWorkspace`: remove the `agentID` argument to `db.NewThread`.

### `internal/agent/runner_test.go`

- Add `TestRunner_RunThread_NoRouter`: verifies `RunThread` returns an error when the workspace has no router agent configured.
- Add `TestRunner_RunThread_RoutesViaWorkspaceRouter`: sets up a workspace with a router agent and verifies `RunThread` calls the provider with the thread history.

### `internal/command/command_test.go`

- Update any `new_thread` test invocations to remove the `agent_id` param.

### `internal/connection/connection_test.go`

- No changes needed (no direct thread tests).

---

## What does NOT change

- `Chat` command signature (`chat <thread_id> <message...>`) — unchanged.
- `ls_threads <workspace_id>` command signature — unchanged.
- `thread_history <thread_id>` command — unchanged.
- File persistence format (WAL, snapshots, message logs) — unchanged.
- Events emitted (`EventAgentRunStarted`, `EventAgentRunCompleted`, `EventAgentRunFailed`) — unchanged (just different agent ID value).

---

## Error behaviour

| Scenario | Error |
|----------|-------|
| `new_thread` with invalid workspace KSUID | `invalid workspace_id: ...` |
| `new_thread` with nonexistent workspace | `workspace not found` |
| `chat` on thread whose workspace has no router configured | `workspace has no router agent configured` |
| `chat` on thread whose workspace has no catalog/agents | routes successfully (router gets empty catalog listing suffix) |
