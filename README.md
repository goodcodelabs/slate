# Slate

A TCP server for LLM agent orchestration. Slate lets you define agents, organize them into catalogs, wire them into multi-agent pipelines, and chat with them over a simple line-oriented protocol — all without HTTP overhead.

## Overview

Slate is built around a small set of core primitives:

| Primitive | Description |
|-----------|-------------|
| **Workspace** | Isolation boundary. Groups a catalog, router agent, threads, and pipelines. |
| **Catalog** | Registry of agent definitions available to a workspace. |
| **Agent** | An LLM-backed actor with instructions, a model, and optional tools. |
| **Thread** | A persistent multi-turn conversation. Workspace threads route through a router agent; agent threads are bound directly to a single agent. |
| **Pipeline** | An ordered sequence of agent steps (sequential or parallel). |
| **Job** | An async unit of work created when a pipeline is run. |

## Architecture

```
TCP Client
    │  newline-delimited JSON protocol
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

The wire protocol is newline-delimited JSON:

- **Request**: `{"cmd":"<name>","params":{...}}\n` (cmd is case-insensitive)
- **Response (success)**: `{"ok":true,"data":{...}}\n` or `{"ok":true}\n`
- **Response (error)**: `{"ok":false,"error":"<message>"}\n`

```
→ {"cmd":"health"}\n
← {"ok":true}\n

→ {"cmd":"add_workspace","params":{"name":"my-project"}}\n
← {"ok":true}\n

→ {"cmd":"add_catalog","params":{"name":"agents"}}\n
← {"ok":true}\n

→ {"cmd":"ls_catalogs"}\n
← {"ok":true,"data":{"catalogs":[{"id":"...","name":"agents"}]}}\n

→ {"cmd":"add_agent","params":{"catalog_id":"<id>","name":"summarizer"}}\n
← {"ok":true,"data":{"id":"...","name":"summarizer"}}\n
```

## Commands

All params and responses are JSON objects. The tables below show the `params` fields and the `data` field of a successful response.

### Workspace

| Command | Params | Response data |
|---------|--------|---------------|
| `ls_workspaces` | — | `{"workspaces":[...]}` |
| `add_workspace` | `{"name":"..."}` | — |
| `del_workspace` | `{"name":"..."}` | — |
| `set_workspace_catalog` | `{"workspace_id":"...","catalog_id":"..."}` | — |
| `set_workspace_router` | `{"workspace_id":"...","agent_id":"..."}` | — |

### Catalog

| Command | Params | Response data |
|---------|--------|---------------|
| `add_catalog` | `{"name":"..."}` | — |
| `del_catalog` | `{"name":"..."}` | — |
| `ls_catalogs` | — | `{"catalogs":[{"id":"...","name":"..."},...]}` |

### Agent

| Command | Params | Response data |
|---------|--------|---------------|
| `add_agent` | `{"catalog_id":"...","name":"..."}` | `{"id":"...","name":"..."}` |
| `del_agent` | `{"agent_id":"..."}` | — |
| `set_agent_instructions` | `{"agent_id":"...","instructions":"..."}` | — |
| `set_agent_model` | `{"agent_id":"...","model":"..."}` | — |
| `run_agent` | `{"agent_id":"...","input":"..."}` | `{"job_id":"..."}` |

### Tools

| Command | Params | Response data |
|---------|--------|---------------|
| `add_tool` | `{"agent_id":"...","tool":"<name>"}` | — |
| `remove_tool` | `{"agent_id":"...","tool":"<name>"}` | — |
| `ls_tools` | `{"agent_id":"..."}` | `{"tools":["..."]}` |

### Workspace Threads

Persistent multi-turn conversations routed through the workspace's router agent.

| Command | Params | Response data |
|---------|--------|---------------|
| `new_thread` | `{"workspace_id":"...","name":"..."}` | `{"id":"...","name":"..."}` |
| `chat` | `{"thread_id":"...","message":"..."}` | `{"job_id":"..."}` |
| `ls_threads` | `{"workspace_id":"..."}` | `{"threads":[...]}` |
| `thread_history` | `{"thread_id":"..."}` | `{"messages":[...]}` |

### Agent Threads

Persistent multi-turn conversations bound directly to a single agent (no workspace or routing required).

| Command | Params | Response data |
|---------|--------|---------------|
| `new_agent_thread` | `{"agent_id":"...","name":"..."}` | `{"id":"...","name":"..."}` |
| `agent_chat` | `{"thread_id":"...","message":"..."}` | `{"job_id":"..."}` |
| `ls_agent_threads` | `{"agent_id":"..."}` | `{"threads":[...]}` |
| `agent_thread_history` | `{"thread_id":"..."}` | `{"messages":[...]}` |

### Pipelines

| Command | Params | Response data |
|---------|--------|---------------|
| `create_pipeline` | `{"workspace_id":"...","name":"..."}` | `{"id":"...","name":"..."}` |
| `add_pipeline_step` | `{"pipeline_id":"...","agent_id":"...","mode":"sequential\|parallel"}` | — |
| `run_pipeline` | `{"pipeline_id":"...","input":"..."}` | `{"job_id":"..."}` |

### Jobs

All async commands (`run_agent`, `chat`, `agent_chat`, `run_pipeline`) return a `job_id` immediately. Poll or wait for results:

| Command | Params | Response data |
|---------|--------|---------------|
| `job_status` | `{"job_id":"..."}` | `{"status":"...","created_at":"...",...}` |
| `job_result` | `{"job_id":"..."}` | `{"status":"...","result":"...","error":"..."}` |
| `wait_job` | `{"job_id":"..."}` | `{"status":"...","result":"...","error":"..."}` |
| `ls_jobs` | `{"workspace_id":"..."}` (optional) | `[{...},...]` |
| `cancel_job` | `{"job_id":"..."}` | — |

`wait_job` blocks until the job reaches a terminal state (completed or failed).

### Management

| Command | Params | Response data |
|---------|--------|---------------|
| `health` | — | — |
| `system_metrics` | — | JSON metrics snapshot |
| `system_stats` | — | JSON combined stats |

### External Agent Registration

A process registers itself as an agent using its own TCP connection with the JSON protocol:

```
→ {"cmd":"register_agent","params":{"catalog_id":"...","name":"...","instructions":"..."}}\n
← {"ok":true,"data":{"agent_id":"..."}}\n
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

// Async agent run — returns a job ID immediately.
jobID, _ := c.RunAgent(info.ID, "Summarize: the quick brown fox...")
result, _ := c.WaitJob(jobID)  // blocks until done
fmt.Println(result.Result)

// Agent thread — persistent conversation bound to a single agent.
thread, _ := c.NewAgentThread(info.ID, "my-convo")
jobID, _ = c.AgentChat(thread.ID, "Hello!")
result, _ = c.WaitJob(jobID)

// Pipeline.
pipeline, _ := c.CreatePipeline(wsID, "research-pipeline")
c.AddPipelineStep(pipeline.ID, agentID, "sequential")
jobID, _ = c.RunPipeline(pipeline.ID, "input text")
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
