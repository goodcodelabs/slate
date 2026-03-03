# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build      # compile to bin/slate-server
make run        # build and start the server (localhost:4242)
make test       # go test ./...
make integrate  # run integration suite (requires a running server: make run)
make clean      # remove bin/

# Run a single test or package
go test ./internal/data/... -run TestAddWorkspace_Success
go test ./internal/agent/... -run TestRunner_Run_SimpleTextResponse -v
```

No linter is configured. No testify — all tests use the standard library `testing` package only.

The integration suite (`cmd/integration/`) requires a live server and an `ANTHROPIC_API_KEY`. Unit tests have no external dependencies.

## Architecture

### Wire protocol

Clients connect over TCP (default port 4242). Each request is a space-separated line ending with `\n`; the server replies with a single line. Errors are prefixed `error|`. The connection handler (`internal/connection/connection.go`) reads requests in a loop, dispatches normal commands onto the scheduler, and short-circuits `quit` and `register_agent` inline.

### Request path

```
TCP line → connection.Handler
             → parser.ParseRequest (lowercase first token, rest = params)
             → scheduler.Schedule (async worker pool, default 4 goroutines)
                 → command.InitCommands[name].Execute
                     → data.Data  (in-memory state + FileStore persistence)
                     → agent.Runner  (LLM + tool loop)
                 → h.Respond(result)
```

`register_agent` bypasses the scheduler entirely: it creates the agent in the store, writes the agent ID back, then blocks on the connection until the server shuts down or the agent disconnects.

### Data layer (`internal/data/`)

`data.Data` is the in-memory state (exported maps: `Workspaces`, `Catalogs`, `Threads`, `Pipelines`, `Jobs`). It delegates all mutations to a `FileStore`:

- **Snapshots**: each entity is written atomically as a msgpack file (`snapshots/<type>/<id>.msgpack`).
- **WAL**: every mutation is also appended as a JSON line to `wal/operations.log`. On startup the snapshots are loaded first, then the WAL is replayed on top to catch any writes since the last checkpoint.
- **Thread messages**: stored separately in append-only JSON-line logs (`snapshots/threads/<id>.log`), never in the snapshot.
- **Jobs**: ephemeral — not persisted, always empty on restart.

WAL entries use msgpack to encode entity payloads (`data` field) inside a JSON envelope.

### Agent runner (`internal/agent/runner.go`)

`Runner.Run` manages the agentic loop: it appends the user message, calls the LLM provider, checks `stop_reason`. If `stop_reason == "tool_use"` it dispatches every tool-use block via `tools.Registry.Execute`, appends the results as a user turn, and loops. The loop terminates on any other stop reason or when no tool results are produced.

`Runner.RunWithOptions` accepts a `SystemPromptSuffix` that is appended to the agent's instructions with a blank-line separator. `RunWorkspaceChat` (router) uses this to inject the catalog listing.

External agents (`agent.External == true`) skip the LLM entirely: `Runner.Run` forwards the input to the registered `AgentConn` and waits for its response.

### Pipeline execution (`internal/agent/pipeline.go`)

Steps are grouped into consecutive runs of the same `StepMode`. A sequential step advances `current` output one at a time. A group of consecutive parallel steps all receive the same `current` input, run concurrently via a `WaitGroup`, and their outputs are joined with `"\n---\n"`. The first error in any group aborts the pipeline.

### Command package (`internal/command/`)

`InitCommands` returns a `map[string]ProtocolCommand`. Commands are thin: they parse/validate params, call one method on `data.Data` or `agent.Runner`, and format the response as a JSON string or `"ok"`. The `command.Context` carries `IPAddress` and `SessionID` but most commands ignore it.

### Metrics and events

`metrics.Metrics` tracks aggregate counters (LLM calls, tool calls, errors, active connections, token totals, latency). `events.Logger` writes per-workspace audit events as JSON lines to `events/<workspace_id>.log`. Both are optional; nil checks guard every call site.

### ID scheme

All entity IDs are `ksuid.KSUID` (20-byte, K-sortable). Map keys throughout `data.Data` are `ksuid.KSUID` values, not strings.

## Key conventions

- `data.New("name", dataDir)` is the correct way to create a store in tests; it wires up the FileStore automatically. Use `t.TempDir()` as `dataDir`.
- `db.Workspaces`, `db.Catalogs`, etc. are exported and can be read directly in tests to get entity IDs without needing a separate lookup method.
- `connection.New` requires a non-nil `*agent.ExternalAgentRegistry`; pass `agent.NewExternalAgentRegistry()` in tests.
- `getConfiguration` in connection uses bitwise OR (`|`) not logical OR — `0 | 6000 == 6000` is the default idle timeout.
- `parser.ParseRequest("")` returns `{Command:"", Params:[]}` (not an error) because `strings.Split("", " ")` returns `[""]`.
