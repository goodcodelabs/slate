# Slate: LLM Agent Orchestration Service — Implementation Plan

## Overview

Slate is already well-positioned for this transformation. It has a TCP server with session management, a durable WAL-backed file store, a command dispatch framework, a scheduler, and the Anthropic Go SDK already in `go.mod`. The goal is to evolve these primitives into a first-class orchestration layer for LLM-based agents.

The architecture centers on three existing concepts:
- **Workspace** → an isolated execution environment for a multi-agent system
- **Catalog** → a registry of reusable agent definitions
- **Agent** → an LLM-backed actor with instructions, tools, and state

---

## Phase 1: LLM Execution Engine

**Goal**: Make individual agents runnable — able to receive a prompt and produce a response backed by an LLM.

### 1.1 — Agent Model Expansion

Extend `internal/data/types.go` to flesh out the `Agent` struct:

```go
type Agent struct {
    ID           ksuid.KSUID
    Name         string
    Instructions string         // system prompt
    Model        string         // e.g. "claude-opus-4-6"
    Tools        []ToolDef      // tool definitions available to this agent
    MaxTokens    int
    Temperature  float64
    Metadata     map[string]string
}

type ToolDef struct {
    Name        string
    Description string
    InputSchema json.RawMessage  // JSON Schema for input validation
}
```

### 1.2 — LLM Provider Interface

Create `internal/llm/` with a provider abstraction:

```
internal/llm/
├── provider.go     # Provider interface
├── anthropic.go    # Anthropic SDK implementation
└── types.go        # Message, ToolCall, ToolResult types
```

**Provider interface**:
```go
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

type CompletionRequest struct {
    Model        string
    SystemPrompt string
    Messages     []Message
    Tools        []ToolDef
    MaxTokens    int
}
```

The Anthropic implementation wraps `github.com/anthropics/anthropic-sdk-go` (already a dependency).

### 1.3 — Agent Runner

Create `internal/agent/runner.go`:

```go
type Runner struct {
    provider llm.Provider
    store    data.Store
}

func (r *Runner) Run(ctx context.Context, agentID ksuid.KSUID, input string, history []llm.Message) (*RunResult, error)
```

The runner handles the agent loop:
1. Load agent definition from store
2. Build system prompt from agent instructions
3. Call LLM provider
4. If response contains tool calls → dispatch to tool executor, append results, loop
5. Return final text response + updated history

### New commands to expose via the protocol:
- `run_agent <agent_id> <input>` — single-turn run
- `add_agent <catalog_id> <name>` — create agent in catalog
- `set_agent_instructions <agent_id> <instructions>`
- `set_agent_model <agent_id> <model>`

---

## Phase 2: Conversation & Context Management

**Goal**: Track multi-turn conversations (chat history) tied to workspaces and agents.

### 2.1 — Chat / Thread Model

The `Workspace` type already has a placeholder `Chats` field. Formalize it:

```go
type Thread struct {
    ID        ksuid.KSUID
    Name      string
    AgentID   ksuid.KSUID
    Messages  []llm.Message
    State     ThreadState   // active, completed, error
    CreatedAt time.Time
    UpdatedAt time.Time
}

type ThreadState string
const (
    ThreadActive    ThreadState = "active"
    ThreadCompleted ThreadState = "completed"
    ThreadError     ThreadState = "error"
)
```

Threads live under a Workspace. Message history is append-only.

### 2.2 — Persistence

Extend the `Store` interface:
```go
SaveThread(t *Thread) error
DeleteThread(id ksuid.KSUID) error
AppendMessage(threadID ksuid.KSUID, msg llm.Message) error
```

Add WAL operation types: `ADD_THREAD`, `REMOVE_THREAD`, `APPEND_MESSAGE`.

Message history can grow large — store thread messages in a separate per-thread append log under `snapshots/threads/<id>/messages.log` to avoid rewriting entire objects.

### 2.3 — New commands:
- `new_thread <workspace_id> <agent_id> [name]`
- `chat <thread_id> <message>` — send a message, get a reply
- `ls_threads <workspace_id>`
- `thread_history <thread_id>`

---

## Phase 3: Tool Framework

**Goal**: Give agents tools they can invoke. Tools are functions the orchestrator executes on the agent's behalf.

### 3.1 — Built-in Tool Registry

Create `internal/tools/`:

```
internal/tools/
├── registry.go     # Tool registry, dispatch
├── types.go        # Tool interface
├── builtin/
│   ├── http.go     # HTTP fetch tool
│   ├── shell.go    # Shell command tool (sandboxed)
│   ├── files.go    # File read/write tool
│   └── agent.go    # Call another agent (handoff)
```

**Tool interface**:
```go
type Tool interface {
    Definition() llm.ToolDef
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

### 3.2 — Agent-as-Tool (Handoff)

The most important built-in: an agent can call another agent. This is the core of multi-agent orchestration. When the LLM calls the `call_agent` tool:
1. Look up the target agent in the workspace catalog
2. Run the target agent with the provided input
3. Return the result as the tool response

### 3.3 — Tool Attachment to Agents

Add commands:
- `add_tool <agent_id> <tool_name> [config_json]`
- `remove_tool <agent_id> <tool_name>`
- `ls_tools <agent_id>`

---

## Phase 4: Multi-Agent Routing & Orchestration Patterns

**Goal**: Enable coordinated multi-agent workflows within a workspace.

### 4.1 — Router Agent

`WorkspaceConfig` already has a `RouterAgentConfig` field. Implement it:

- Each workspace can have a designated **router agent**
- When a message is sent to the workspace (not to a specific agent), the router decides which agent(s) to invoke
- Router uses the workspace's catalog as its tool list (each catalog agent becomes a callable tool)

Router invocation via command:
- `workspace_chat <workspace_id> <message>` — routes through the workspace router

### 4.2 — Orchestration Patterns

Implement standard patterns as first-class primitives:

| Pattern | Description |
|---------|-------------|
| **Sequential** | Agent A → Agent B → Agent C in order |
| **Parallel** | Fan out to N agents, collect results |
| **Handoff** | Agent A delegates a subtask to Agent B mid-run |
| **Supervisor** | A supervisor agent reviews and corrects subordinate output |

These can be modeled as pipeline definitions stored in the workspace:

```go
type Pipeline struct {
    ID    ksuid.KSUID
    Name  string
    Steps []PipelineStep
}

type PipelineStep struct {
    AgentID  ksuid.KSUID
    Mode     StepMode   // sequential, parallel
    InputMap string     // jq-style expression mapping prior output to this step's input
}
```

New commands:
- `create_pipeline <workspace_id> <name>`
- `add_pipeline_step <pipeline_id> <agent_id> <mode>`
- `run_pipeline <pipeline_id> <input>`

### 4.3 — Async Job Queue

The existing `Scheduler` is synchronous (single goroutine, capacity 64). For LLM workloads this needs to become a proper async job system:

- Replace buffered channel with a priority work queue
- Jobs have: ID, type, input, status, result, created/started/completed timestamps
- Persist job state to store (survives restarts)
- Expose job polling via commands: `job_status <job_id>`, `job_result <job_id>`
- Long-running orchestrations become jobs clients can poll

---

## Phase 5: Observability & Management

**Goal**: Understand what agents are doing at runtime.

### 5.1 — Structured Logging

Current logging uses `log/slog` — already structured. Extend to include:
- `workspace_id`, `agent_id`, `thread_id` in log context
- LLM call logs: model, input tokens, output tokens, latency, tool calls
- Tool execution logs: tool name, input, output, latency

### 5.2 — Metrics

Create `internal/metrics/`:
- Counters: total LLM calls, tool calls, errors
- Histograms: LLM latency, token counts, pipeline durations
- Expose via the TCP command interface (e.g. `system_metrics`) as structured JSON

### 5.3 — Event Stream

Persist an event log per workspace — a chronological record of everything that happened:
- Agent runs started/completed
- Tool calls
- Messages exchanged
- Errors

This doubles as an audit trail and a replay mechanism.

### 5.4 — Management Commands

- `system_stats` — active connections, queued jobs, storage usage
- `ls_jobs [workspace_id]` — list running/pending jobs
- `cancel_job <job_id>` — cancel a running job (requires context cancellation)

---

## Phase 6: Agent SDK & Client Libraries

**Goal**: Make it easy to build agents that connect to Slate.

### 6.1 — Go Client Library

Create `pkg/client/`:
```go
type Client struct { ... }

func (c *Client) NewThread(ctx context.Context, workspaceID, agentID string) (*Thread, error)
func (c *Client) Chat(ctx context.Context, threadID, message string) (*Message, error)
func (c *Client) RunPipeline(ctx context.Context, pipelineID, input string) (*Job, error)
```

### 6.2 — Agent Registration Protocol

Agents can be external processes that register themselves over TCP:
1. External process connects
2. Sends `register_agent <name> <instructions>` + tool definitions
3. Receives an agent ID
4. Orchestrator calls back to this connection when the agent needs to run
5. Agent processes the prompt and returns a response

This enables polyglot agents (Python, TypeScript, etc.) running as separate processes.

---

## Implementation Order & Dependencies

```
Phase 1 (LLM Engine)
    ↓
Phase 2 (Threads)  ←→  Phase 3 (Tools)
         ↘         ↙
       Phase 4 (Routing)
            ↓
Phase 5 (Observability)  ←→  Phase 6 (SDK)
```

Recommended milestone sequence:

1. **M1** — Phase 1: Run a single agent against the Anthropic API end-to-end
2. **M2** — Phase 2 + 3: Multi-turn threads and built-in tools (agent handoff)
3. **M3** — Phase 4: Router agent and pipeline execution
4. **M4** — Phase 5 + 6: Observability, metrics, client SDK

---

## Key Design Decisions

### Agent State Model
Agents are **stateless definitions** (instructions + model + tools). State lives in threads. This keeps agents reusable across contexts and avoids coupling execution state to agent identity.

### Workspace as Isolation Boundary
A workspace is the unit of isolation: its own catalog of agents, its own threads, its own pipeline definitions. Multi-tenant scenarios map naturally — one workspace per customer/project.

### Persistence Strategy
Continue using the WAL + snapshot model. Add per-thread append logs to handle high-volume message history without rewriting entire snapshot files.

### LLM Provider Abstraction
The `llm.Provider` interface must be pluggable from day one. Anthropic is the default (SDK already included), but the interface should not leak Anthropic-specific types.

### Concurrency Model
Move from single-goroutine scheduler to a bounded goroutine pool for LLM calls (which are I/O-bound and can run concurrently). Use `context.Context` throughout for cancellation.

---

## Files to Create / Modify

### New packages
| Path | Purpose |
|------|---------|
| `internal/llm/` | LLM provider interface + Anthropic impl |
| `internal/agent/` | Agent runner, loop logic |
| `internal/tools/` | Tool registry + built-ins |
| `internal/pipeline/` | Pipeline definition + execution |
| `internal/metrics/` | Metrics collection |
| `pkg/client/` | Public Go client library |

### Modify existing
| Path | Change |
|------|--------|
| `internal/data/types.go` | Expand Agent, add Thread, Pipeline, Job types |
| `internal/data/store.go` | Add Thread, Pipeline, Job methods to Store interface |
| `internal/data/filestore.go` | Implement new store methods |
| `internal/data/wal.go` | Add new WAL operation types |
| `internal/command/command.go` | Register new commands |
| `internal/scheduler/scheduler.go` | Upgrade to async pool with job persistence |
