# Plan: Agent Threads

## Problem

Workspace threads route every message through the workspace's router agent. This works well for multi-agent orchestration, but there are use cases where you want a persistent conversation with a specific agent directly — no routing, no workspace required.

## Goal

Add `AgentThread` as a first-class entity: a persistent, multi-turn conversation bound to a single agent. Creating one requires only an `agent_id`. Chatting on one calls that agent directly with the full accumulated history.

---

## Data Model

### 1. `internal/data/types.go`

Add `AgentThread`:

```go
type AgentThread struct {
    ID        ksuid.KSUID   `msgpack:"id"`
    AgentID   ksuid.KSUID   `msgpack:"agent_id"`
    Name      string        `msgpack:"name"`
    State     ThreadState   `msgpack:"state"`    // reuse existing ThreadState constants
    CreatedAt time.Time     `msgpack:"created_at"`
    UpdatedAt time.Time     `msgpack:"updated_at"`

    // Messages are loaded from the per-thread log, not serialized in the snapshot.
    Messages  []llm.Message `msgpack:"-"`
}
```

Also add `AgentThreads map[ksuid.KSUID]*AgentThread` to `Data`.

---

## Persistence

### 2. `internal/data/wal.go`

Add two new op codes:

```go
OpAddAgentThread    OperationType = "ADD_AGENT_THREAD"
OpRemoveAgentThread OperationType = "REMOVE_AGENT_THREAD"
```

Add cases to `Replay` to handle these ops, deserializing into `AgentThread` and inserting into/removing from `data.AgentThreads`.

### 3. `internal/data/filestore.go`

Add a new storage directory constant:

```go
agentThreadsDir = "snapshots/agent_threads"
```

Add `agentThreadsDir` to `initDirectories`.

Add four new `FileStore` methods following the exact same pattern as threads:

```go
// SaveAgentThread persists metadata to WAL + snapshot.
func (fs *FileStore) SaveAgentThread(t *AgentThread) error

// DeleteAgentThread removes snapshot and message log.
func (fs *FileStore) DeleteAgentThread(id ksuid.KSUID) error

// AppendAgentMessage appends a message to the agent thread's JSON-line log.
func (fs *FileStore) AppendAgentMessage(threadID ksuid.KSUID, msg llm.Message) error
```

Add private helpers mirroring the existing thread loaders:

```go
func (fs *FileStore) loadAgentThreadSnapshots(data *Data) error
func (fs *FileStore) loadAgentThreadMessages(data *Data) error
```

Both are called from `Load`, following the same order as their thread counterparts (snapshots first, WAL replay, then messages).

### 4. `internal/data/store.go`

Add the new methods to the `Store` interface:

```go
SaveAgentThread(t *AgentThread) error
DeleteAgentThread(id ksuid.KSUID) error
AppendAgentMessage(threadID ksuid.KSUID, msg llm.Message) error
```

---

## Data Layer

### 5. `internal/data/data.go`

Initialize `AgentThreads` map in `New` (always fresh-allocated after `Load`).

Add four data methods:

```go
// NewAgentThread creates and persists an agent thread.
// Returns an error if the agent does not exist.
func (db *Data) NewAgentThread(agentID ksuid.KSUID, name string) (*AgentThread, error)

func (db *Data) GetAgentThread(id ksuid.KSUID) (*AgentThread, error)

func (db *Data) DeleteAgentThread(id ksuid.KSUID) error

// ListAgentThreads returns all threads for the given agent.
func (db *Data) ListAgentThreads(agentID ksuid.KSUID) ([]*AgentThread, error)

// AppendAgentMessage appends a message to an agent thread's history.
func (db *Data) AppendAgentMessage(threadID ksuid.KSUID, msg llm.Message) error
```

`NewAgentThread` validates the agent exists via `FindAgent` before creating the thread.

---

## Runner

### 6. `internal/agent/runner.go`

Add `RunAgentThread` to the `Runner`:

```go
func (r *Runner) RunAgentThread(ctx context.Context, threadID ksuid.KSUID, input string) (*RunResult, error) {
    thread, err := r.store.GetAgentThread(threadID)
    // ...

    historyLen := len(thread.Messages)

    result, err := r.Run(ctx, thread.AgentID, input, thread.Messages)
    // ... emit events, persist new messages via AppendAgentMessage ...

    return result, nil
}
```

Events use the same `EventAgentRunStarted/Completed/Failed` types as `RunThread`.
No `SystemPromptSuffix` — the agent's own instructions are used as-is.

---

## Commands

### 7. `internal/command/` — new file `agent_thread.go`

Four commands:

| Command | Signature | Description |
|---------|-----------|-------------|
| `new_agent_thread` | `new_agent_thread <agent_id> [name...]` | Creates a thread for an agent |
| `agent_chat` | `agent_chat <thread_id> <message...>` | Sends a message; returns response |
| `ls_agent_threads` | `ls_agent_threads <agent_id>` | Lists threads for an agent |
| `agent_thread_history` | `agent_thread_history <thread_id>` | Returns the full message history |

`new_agent_thread` response: `{"id":"...","name":"..."}`

`ls_agent_threads` response:
```json
{
  "threads": [
    {
      "id": "...",
      "name": "...",
      "agent_id": "...",
      "state": "active",
      "messages": 4,
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

`agent_thread_history` response: `{"messages": [...]}`

### 8. `internal/command/command.go`

Add four constants and four entries to `InitCommands`:

```go
CmdNewAgentThread        = "new_agent_thread"
CmdAgentChat             = "agent_chat"
CmdListAgentThreads      = "ls_agent_threads"
CmdAgentThreadHistory    = "agent_thread_history"
```

---

## Client

### 9. `pkg/client/types.go`

Add `AgentThreadInfo`:

```go
type AgentThreadInfo struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    AgentID   string `json:"agent_id"`
    State     string `json:"state"`
    Messages  int    `json:"messages"`
    CreatedAt string `json:"created_at"`
    UpdatedAt string `json:"updated_at"`
}
```

### 10. `pkg/client/client.go`

Add four methods:

```go
func (c *Client) NewAgentThread(agentID string, name string) (AgentThreadInfo, error)
func (c *Client) AgentChat(threadID, message string) (string, error)
func (c *Client) ListAgentThreads(agentID string) ([]AgentThreadInfo, error)
func (c *Client) AgentThreadHistory(threadID string) (string, error)
```

---

## Tests

### `internal/data/data_test.go`

- `TestNewAgentThread_Success` — creates a thread, checks ID and AgentID
- `TestNewAgentThread_AgentNotFound` — expects error for unknown agent
- `TestGetAgentThread_Found` / `TestGetAgentThread_NotFound`
- `TestDeleteAgentThread`
- `TestListAgentThreads_ByAgent`

### `internal/agent/runner_test.go`

- `TestRunner_RunAgentThread_Success` — verifies response and message persistence
- `TestRunner_RunAgentThread_AgentNotFound` — expects error

### `internal/command/command_test.go`

- `TestNewAgentThreadCommand_Success`
- `TestNewAgentThreadCommand_MissingParams`
- `TestAgentChatCommand_MissingParams` (runner is nil for param-validation tests)

---

## Error Behaviour

| Scenario | Error |
|----------|-------|
| `new_agent_thread` with invalid agent KSUID | `invalid agent_id: ...` |
| `new_agent_thread` with nonexistent agent | `agent not found` |
| `agent_chat` with invalid thread KSUID | `invalid thread_id: ...` |
| `agent_chat` on thread whose agent no longer exists | `loading agent: agent not found` |
| `ls_agent_threads` with invalid agent KSUID | `invalid agent_id: ...` |

---

## What Does NOT Change

- Existing `Thread` / workspace threads — untouched.
- `RunThread` routing logic — untouched.
- WAL replay for existing op codes — untouched.
- `Store` interface methods for existing entities — untouched.
