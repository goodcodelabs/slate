# Slate

A TCP server for LLM agent orchestration. Slate lets you define agents, organize them into catalogs, wire them into multi-agent pipelines, and chat with them over a simple line-oriented protocol — all without HTTP overhead.

## Overview

Slate is built around a small set of core primitives:

| Primitive | Description |
|-----------|-------------|
| **Workspace** | Isolation boundary. Groups a catalog, router agent, threads, and pipelines. |
| **Catalog** | Registry of agent definitions available to a workspace. |
| **Agent** | An LLM-backed actor with instructions, a model, and optional tools. |
| **Thread** | A persistent multi-turn conversation tied to a workspace and agent. |
| **Pipeline** | An ordered sequence of agent steps (sequential or parallel). |
| **Job** | An async unit of work created when a pipeline is run. |

## Architecture

```
TCP Client
    │  line-oriented protocol (space-separated tokens, \n terminated)
    ▼
connection.Handler
    │  parses request, dispatches via scheduler
    ▼
command.*Command  ──► data.Data  ──► FileStore (msgpack snapshots + WAL)
    │
    └──► agent.Runner  ──► llm.Provider (Anthropic)
              │
              ├──► tools.Registry (http_fetch, shell, file, call_agent)
              ├──► RunPipeline (parallel/sequential step groups)
              └──► ExternalAgentRegistry (registered external processes)
```

### Persistence

Data is stored in a directory (default `./data`) with the following layout:

```
data/
  metadata.json                    # store version and checkpoint info
  wal/operations.log               # write-ahead log (JSON lines)
  snapshots/
    workspaces/<id>.msgpack        # workspace snapshots (msgpack)
    catalogs/<id>.msgpack          # catalog + agent snapshots
    threads/<id>.msgpack           # thread metadata snapshots
    threads/<id>.log               # per-thread message history (JSON lines)
    pipelines/<id>.msgpack         # pipeline snapshots
  events/<workspace_id>.log        # per-workspace audit log (JSON lines)
```

On startup: snapshots are loaded, then the WAL is replayed on top. On shutdown: WAL is flushed and closed. Every 1000 operations a checkpoint is taken (WAL truncated, snapshots written).

### Scheduler

Commands are executed by a worker pool (default 4 goroutines). Long-running commands (pipeline runs, LLM calls) run without blocking the connection read loop.

## Protocol

The wire protocol is line-oriented:

- **Request**: space-separated tokens followed by `\n`. First token is the command name (case-insensitive). Remaining tokens are parameters.
- **Response**: a single line followed by `\n`. Errors are prefixed with `error|`.

```
→ health\n
← ok\n

→ add_workspace my-project\n
← ok\n

→ add_catalog agents\n
← ok\n

→ ls_catalogs\n
← {"catalogs":[{"id":"...","name":"agents"}]}\n

→ add_agent <catalog_id> summarizer\n
← {"id":"...","name":"summarizer"}\n
```

## Commands

### Workspace

| Command | Params | Response |
|---------|--------|----------|
| `add_workspace` | `<name>` | `ok` |
| `del_workspace` | `<name>` | `ok` |
| `set_workspace_catalog` | `<workspace_id> <catalog_id>` | `ok` |
| `set_workspace_router` | `<workspace_id> <agent_id>` | `ok` |
| `workspace_chat` | `<workspace_id> <message...>` | assistant reply |

### Catalog

| Command | Params | Response |
|---------|--------|----------|
| `add_catalog` | `<name>` | `ok` |
| `del_catalog` | `<name>` | `ok` |
| `ls_catalogs` | — | `{"catalogs":[...]}` |

### Agent

| Command | Params | Response |
|---------|--------|----------|
| `add_agent` | `<catalog_id> <name>` | `{"id":"...","name":"..."}` |
| `set_agent_instructions` | `<agent_id> <instructions...>` | `ok` |
| `set_agent_model` | `<agent_id> <model>` | `ok` |
| `run_agent` | `<agent_id> <input...>` | assistant reply |

### Tools

| Command | Params | Response |
|---------|--------|----------|
| `add_tool` | `<agent_id> <tool_name>` | `ok` |
| `remove_tool` | `<agent_id> <tool_name>` | `ok` |
| `ls_tools` | `<agent_id>` | `{"tools":["..."]}` |

### Threads

| Command | Params | Response |
|---------|--------|----------|
| `new_thread` | `<workspace_id> <agent_id> [name]` | `{"id":"...","name":"..."}` |
| `chat` | `<thread_id> <message...>` | assistant reply |
| `ls_threads` | `<workspace_id>` | `{"threads":[...]}` |
| `thread_history` | `<thread_id>` | `{"messages":[...]}` |

### Pipelines

| Command | Params | Response |
|---------|--------|----------|
| `create_pipeline` | `<workspace_id> <name>` | `{"pipeline_id":"..."}` |
| `add_pipeline_step` | `<pipeline_id> <agent_id> <sequential\|parallel>` | `ok` |
| `run_pipeline` | `<pipeline_id> <input...>` | `{"job_id":"..."}` |

### Jobs

| Command | Params | Response |
|---------|--------|----------|
| `job_status` | `<job_id>` | `{"status":"...","created_at":"...",...}` |
| `job_result` | `<job_id>` | `{"status":"...","result":"...","error":"..."}` |
| `ls_jobs` | `[workspace_id]` | `[{...},...]` |
| `cancel_job` | `<job_id>` | `ok` |

### Management

| Command | Params | Response |
|---------|--------|----------|
| `health` | — | `ok` |
| `system_metrics` | — | JSON metrics snapshot |
| `system_stats` | — | JSON combined stats |

### External Agent Registration

A process can register itself as an agent over its own TCP connection:

```
→ register_agent <catalog_id> <name> <instructions...>\n
← {"agent_id":"..."}\n
```

After registration the connection stays open. The server sends run requests as JSON lines and the process replies with JSON lines:

```
← {"run_id":"...","input":"hello"}\n
→ {"run_id":"...","response":"world"}\n
```

## Built-in Tools

Tools are attached to agents and invoked automatically during the agentic loop when the LLM requests them.

### `http_fetch`

Fetches a URL and returns the HTTP status code and response body (capped at 1 MB).

```json
{"url": "https://example.com", "method": "GET", "headers": {}, "body": ""}
```

### `shell`

Executes a shell command (`sh -c`) and returns combined stdout/stderr. Timeout defaults to 30 s; maximum 120 s.

```json
{"command": "ls -la", "timeout_seconds": 10}
```

### `file`

Reads or writes files on the local filesystem. Path traversal (`..`) is rejected.

```json
{"action": "read",   "path": "/tmp/out.txt"}
{"action": "write",  "path": "/tmp/out.txt", "content": "hello"}
{"action": "append", "path": "/tmp/out.txt", "content": "\nworld"}
```

### `call_agent`

Calls another agent by ID. Registered automatically; allows agents to delegate to other agents. Used by the router in `workspace_chat`.

```json
{"agent_id": "<ksuid>", "input": "summarize this"}
```

## Go Client SDK

`pkg/client` provides a Go client for use in applications and external agents.

```go
import "slate/pkg/client"

// Connect.
c, err := client.Dial("localhost:4242")
defer c.Close()

// Basic operations.
c.AddWorkspace("my-project")
c.AddCatalog("agents")
catalogs, _ := c.ListCatalogs()

catID := catalogs[0].ID
info, _ := c.AddAgent(catID, "summarizer")
c.SetAgentInstructions(info.ID, "You summarize text concisely.")
reply, _ := c.RunAgent(info.ID, "Summarize: the quick brown fox...")

// Pipeline.
ws, _ := c.CreatePipeline(wsID, "research-pipeline")
c.AddPipelineStep(ws.ID, agentID, "sequential")
jobID, _ := c.RunPipeline(ws.ID, "input text")
status, _ := c.JobStatus(jobID)

// External agent registration.
sess, _ := client.DialAgentSession("localhost:4242", catID, "my-agent", "You are helpful.")
defer sess.Close()
sess.Run(ctx, func(ctx context.Context, input string) string {
    return "response: " + input
})
```

## Configuration

All settings are read from environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `localhost` | Bind address |
| `PORT` | `4242` | TCP port |
| `MAX_CONNECTIONS` | `10` | Maximum concurrent client connections |
| `CLIENT_IDLE_TIMEOUT_MS` | `60000` | Client idle timeout in milliseconds |
| `WORKERS` | `4` | Scheduler worker goroutines |
| `DATA_DIR` | `./data` | Directory for snapshots, WAL, and event logs |

An `ANTHROPIC_API_KEY` environment variable is required for LLM calls.

## Getting Started

```bash
# Build and run the server.
make run

# Run unit tests.
make test

# Run integration tests (requires a running server).
make integrate

# Build only.
make build

# Remove build artifacts.
make clean
```

## Project Layout

```
cmd/
  server/           # server binary entry point + configuration
  integration/      # integration test suite (uses pkg/client)
internal/
  agent/            # runner, pipeline execution, router, external agent registry
  command/          # command implementations (one file per domain)
  connection/       # TCP connection handler and dispatcher
  data/             # data models, in-memory store, WAL, file store
  events/           # per-workspace audit event logger
  llm/              # LLM provider interface + Anthropic implementation
  metrics/          # runtime metrics (counters, latency)
  parser/           # request line parser
  scheduler/        # bounded worker pool
  tools/            # tool registry + built-in tools (http, shell, file)
pkg/
  client/           # Go client library for the Slate protocol
plans/              # implementation plans and design notes
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/anthropics/anthropic-sdk-go` | Anthropic LLM API |
| `github.com/segmentio/ksuid` | K-sortable unique IDs |
| `github.com/vmihailenco/msgpack/v5` | Binary serialization for snapshots |
