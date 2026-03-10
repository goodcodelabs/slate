# Interface and Design Improvements

This document identifies concrete problems in the current implementation and recommends targeted changes. Issues are grouped by severity and theme.

---

## 1. Critical: TCP stream framing is broken

**Problem:** `connection.HandleConnection` reads into a fixed 16384-byte buffer with a single `Read` call:

```go
buffer := make([]byte, 16384)
n, err := h.Connection.Read(buffer)
req, err := h.requestParser.ParseRequest(string(buffer[:n]))
```

TCP is a byte stream — one `Read` call does not necessarily return one complete line. A large agent instruction or chat message can arrive in multiple reads, or two short commands can arrive in one read. The current code silently misparses or drops data in both cases.

**Fix:** Replace the raw `Read` with `bufio.Scanner` (or `bufio.Reader.ReadString('\n')`) so the connection loop works on complete newline-terminated frames regardless of how TCP delivers them. The client library already does this correctly with `bufio.Reader`.

---

## 2. Critical: No concurrency protection on the data layer

**Problem:** `data.Data` holds exported maps (`Workspaces`, `Catalogs`, `Threads`, etc.) that are read and written by multiple goroutines concurrently (the scheduler runs 4 workers by default). There is no mutex anywhere in the data layer. This is a data race on every concurrent request pair that touches the same map.

**Fix:** Add a `sync.RWMutex` to `data.Data`. Acquire a read lock for queries (`FindAgent`, `GetThread`, `ListThreads`, etc.) and a write lock for mutations (`AddAgent`, `AppendMessage`, etc.). This is a straightforward mechanical change because all mutations already go through methods.

---

## 3. High: Wire protocol cannot represent structured or multi-line content

**Problem:** The protocol splits every request on spaces. Any parameter with a space in it (agent instructions, chat messages, error text) can only appear as the final "rest of line" argument — joined back with `strings.Join(params[N:], " ")`. This means:

- You cannot pass two separate multi-word parameters in one command.
- Newlines in chat messages are impossible — a newline terminates the command.
- Encoding/escaping rules for the client are undefined and inconsistent.

**Recommendation:** Move to newline-delimited JSON framing. Each request becomes a JSON object:

```json
{"cmd": "chat", "params": {"thread_id": "...", "message": "Hello\nworld"}}
```

Each response is also a JSON object:

```json
{"ok": true, "data": {"job_id": "..."}}
{"ok": false, "error": "thread not found"}
```

This eliminates the fragile space-splitting and `strings.Join` hacks throughout every command handler, removes the need for `error|` prefix conventions, and makes the wire format machine-readable by default. The client library already speaks line-delimited JSON internally (for the external agent protocol) so this is a natural extension.

If a full protocol migration is not desired in one step, a narrower fix is to adopt percent-encoding or base64 for any parameter that may contain spaces or newlines.

---

## 4. High: `InitCommands` is called on every request

**Problem:** Inside `HandleConnection`, the command map is rebuilt from scratch on every request:

```go
// inside the request loop:
commands := command.InitCommands(h.store, h.runner, h.sched, h.metrics)
```

This allocates a new `map[string]ProtocolCommand` (and all of its struct values) on every incoming command. With 4 worker goroutines under load this is wasted allocation and GC pressure.

**Fix:** Build the command map once in `New` (when the handler is constructed) and store it as a field. `InitCommands` already takes all the dependencies it needs.

---

## 5. High: Agent data model — O(N) scans and no delete

**Problem:** Agents are stored as a `[]*Agent` slice inside `Catalog`. `FindAgent` iterates every catalog and every agent on every lookup. Every agent mutation (set instructions, set model, add tool) re-saves the entire catalog as a msgpack file. There is also no command to delete an agent.

**Specific issues:**
- `FindAgent` is O(catalogs × agents) on every agent operation and every LLM run.
- Saving the full catalog to serialize one agent field change is wasteful and will grow linearly with catalog size.
- External agent registration creates a new agent record on every connection — there is no way to reconnect with an existing ID, so catalogs accumulate stale external agent entries on each server restart.

**Recommendations:**
1. Add a flat `Agents map[ksuid.KSUID]*Agent` index to `data.Data`, populated on load, updated on every agent mutation. Use this map for all lookups. Keep the catalog's slice as the canonical storage but maintain the index in sync.
2. Add a `del_agent <agent_id>` command.
3. For external agents: add a `register_agent <existing_agent_id>` form that reconnects to an existing agent record instead of always creating a new one.

---

## 6. Medium: Async job polling is the only option — no blocking wait

**Problem:** Every command that does LLM work (`chat`, `run_agent`, `run_pipeline`, `agent_chat`) returns a `job_id` immediately and the caller must loop on `job_status` until done, then call `job_result`. This is the only option. Real-time clients end up writing the same polling loop every time.

**Recommendation:** Add a `wait_job <job_id>` command that blocks on the server side until the job completes and returns the result in one response. The implementation is straightforward — the job already has a status field; the handler can poll or use a condition variable. Clients that want to do other work can still use the poll approach; clients that want simplicity get a single blocking call.

---

## 7. Medium: `Thread` and `AgentThread` are near-duplicate types

**Problem:** `Thread` (workspace-routed) and `AgentThread` (agent-direct) have identical fields (ID, Name, State, CreatedAt, UpdatedAt, Messages) differing only in their parent reference (`WorkspaceID` vs `AgentID`). This duplication has already spread into: two parallel sets of data methods (`NewThread`/`NewAgentThread`, `AppendMessage`/`AppendAgentMessage`, etc.), two sets of commands (`new_thread`/`new_agent_thread`, `chat`/`agent_chat`, etc.), two sets of FileStore paths, and two sets of client methods.

**Recommendation:** Unify into a single `Thread` type with an optional `AgentID` field. If `AgentID` is set, the thread is agent-direct; otherwise it uses the workspace router. This halves the code paths in the data layer, command layer, and client.

---

## 8. Medium: `getConfiguration` uses bitwise OR instead of conditional default

**Problem:** In `connection.go`:

```go
func getConfiguration(config *Options) *Options {
    return &Options{
        ClientIdleTimeout: config.ClientIdleTimeout | 6000,
    }
}
```

This uses bitwise OR (`|`) to set a default, which is wrong. For example, if `ClientIdleTimeout` is `5000` (binary: `1001110001000`), then `5000 | 6000 = 7160`, not `6000`. The intent is almost certainly `if config.ClientIdleTimeout == 0 { return 6000 }`.

**Fix:**
```go
timeout := config.ClientIdleTimeout
if timeout == 0 {
    timeout = 6000
}
return &Options{ClientIdleTimeout: timeout}
```

---

## 9. Medium: No streaming — LLM responses are fully buffered

**Problem:** The agentic loop in `Runner.Run` runs to completion before returning. For long multi-step tool-use chains, the caller sees nothing until the entire run finishes. This makes the system feel unresponsive for complex tasks and makes it impossible to stream tokens to a UI.

**Recommendation:** Introduce an optional streaming callback in `RunOptions`:

```go
type RunOptions struct {
    SystemPromptSuffix string
    OnToken            func(text string)  // called for each text chunk, if non-nil
    OnToolCall         func(name string, input json.RawMessage)
    OnToolResult       func(name string, output json.RawMessage)
}
```

The Anthropic SDK supports streaming. Adding hook points here doesn't require changing the wire protocol immediately — the streaming data can still be buffered at the command layer until the protocol supports push responses.

---

## 10. Low: Dead code and placeholder types

**Problem:** Several types serve no purpose:

- `Chat struct{}` and `Workspace.Chats map[string]*Chat` — unused, never populated.
- `WorkspaceState struct{}` and `Workspace.State *WorkspaceState` — empty and unused.
- `RouterAgentConfig.Instructions` — the workspace router's instructions are never read; the router agent's own `Agent.Instructions` field is used instead.
- `System` and `SystemConfiguration` types — defined but never used anywhere.

These add noise to the type definitions and serialized snapshots.

**Fix:** Remove them. If workspace-level state is needed in the future it can be added back when there is a concrete use case.

---

## 11. Low: No authentication

**Problem:** The TCP server accepts all connections with no authentication. Any client on the network can read or modify any workspace, catalog, agent, or thread.

**Recommendation:** At minimum add a pre-shared key handshake: the first line of each connection must be `auth <key>` and the server closes the connection if it does not match a configured key. This is simple to implement and appropriate for a server meant to be deployed in a trusted network. A proper multi-tenant auth layer can be layered on top later.

---

## Summary — Recommended Implementation Order

| Priority | Change                                            | Effort |
|----------|---------------------------------------------------|--------|
| 1        | Fix TCP stream framing (`bufio.Scanner`)          | Small  |
| 2        | Add `sync.RWMutex` to `data.Data`                | Small  |
| 3        | Fix `getConfiguration` bitwise OR bug             | Trivial|
| 4        | Build command map once in `connection.New`        | Trivial|
| 5        | Add agent index + `del_agent` command             | Medium |
| 6        | Add `wait_job` blocking command                   | Small  |
| 7        | Unify `Thread`/`AgentThread` types                | Medium |
| 8        | Add pre-shared key authentication                 | Small  |
| 9        | Move to JSON wire protocol                        | Large  |
| 10       | Remove dead placeholder types                     | Trivial|
| 11       | Add streaming callbacks to `RunOptions`           | Medium |
