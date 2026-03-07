# Plan: Non-Blocking LLM Commands

## Problem

Three commands currently block the TCP connection for the full duration of an Anthropic API call:

| Command | Calls | Typical latency |
|---------|-------|----------------|
| `run_agent` | `runner.Run` | 1–30 s |
| `chat` | `runner.RunThread` | 1–30 s |
| `agent_chat` | `runner.RunAgentThread` | 1–30 s |

All three should return a `job_id` immediately, like `run_pipeline` already does. The caller polls `job_status` / `job_result` to retrieve the response.

## Goal

Every command that calls the LLM returns immediately with `{"job_id": "..."}`. The actual work runs on the scheduler's worker pool. The response text is available via `job_result <job_id>` once the job reaches `completed` status.

---

## Changes

### 1. `internal/data/types.go` — Add `ThreadID` to `Job`

Thread-based jobs need a way to correlate a job back to its thread. Add one field to `Job`:

```go
type Job struct {
    ID          ksuid.KSUID        `msgpack:"id"`
    Type        string             `msgpack:"type"`
    WorkspaceID ksuid.KSUID        `msgpack:"workspace_id"`
    PipelineID  ksuid.KSUID        `msgpack:"pipeline_id"`
    ThreadID    ksuid.KSUID        `msgpack:"thread_id"`   // ← new; zero when not a thread job
    Input       string             `msgpack:"input"`
    Status      JobStatus          `msgpack:"status"`
    Result      string             `msgpack:"result"`
    Error       string             `msgpack:"error"`
    CreatedAt   time.Time          `msgpack:"created_at"`
    StartedAt   time.Time          `msgpack:"started_at"`
    CompletedAt time.Time          `msgpack:"completed_at"`
    CancelFunc  context.CancelFunc `msgpack:"-"`
}
```

No migration required — jobs are in-memory only and reset on restart.

---

### 2. `internal/command/jobs.go` — Expose `thread_id` in `ls_jobs`

The `ListJobsCommand` response currently includes `pipeline_id`. Add `thread_id` alongside it so clients can correlate:

```go
// Before
type jobInfo struct {
    ...
    PipelineID string `json:"pipeline_id,omitempty"`
    ...
}

// After
type jobInfo struct {
    ...
    PipelineID string `json:"pipeline_id,omitempty"`
    ThreadID   string `json:"thread_id,omitempty"`
    ...
}
```

Populate `ThreadID` from `j.ThreadID.String()` when non-zero (same pattern as `PipelineID`).

---

### 3. `internal/command/agent.go` — Make `run_agent` async

**`RunAgentCommand`** gains two fields: `store *data.Data` and `sched *scheduler.Scheduler`.

```go
// Before
type RunAgentCommand struct {
    runner *agent.Runner
}

// After
type RunAgentCommand struct {
    store  *data.Data
    runner *agent.Runner
    sched  *scheduler.Scheduler
}
```

`Execute` new logic:

1. Parse `agent_id` and `input` (same as before).
2. Eagerly verify the agent exists via `store.FindAgent(agentID)` — fail immediately for unknown agents, before touching the scheduler.
3. Create a job: `store.CreateJob("agent_run", ksuid.KSUID{}, ksuid.KSUID{}, input)` — no workspace or thread context.
4. Create a cancellable context and attach to job via `store.SetJobCancel`.
5. `sched.Schedule` a closure that:
   - Calls `store.UpdateJob(job.ID, data.JobRunning, "", "")`
   - Calls `runner.Run(jobCtx, agentID, input, nil)`
   - Calls `store.UpdateJob(job.ID, data.JobCompleted/Failed, result.Response, err.Error())`
6. Return `{"job_id": job.ID.String()}`.

---

### 4. `internal/command/thread.go` — Make `chat` async

**`ChatCommand`** gains `store *data.Data` and `sched *scheduler.Scheduler`:

```go
// Before
type ChatCommand struct {
    runner *agent.Runner
}

// After
type ChatCommand struct {
    store  *data.Data
    runner *agent.Runner
    sched  *scheduler.Scheduler
}
```

`Execute` new logic:

1. Parse `thread_id` and `input`.
2. Eagerly load the thread via `store.GetThread(threadID)` — fail immediately if not found.
3. Create job: `store.CreateJob("thread_chat", thread.WorkspaceID, ksuid.KSUID{}, input)`.
4. Set `job.ThreadID = threadID` directly (jobs are in-memory pointers).
5. Create cancellable context, attach via `store.SetJobCancel`.
6. Schedule closure:
   - `store.UpdateJob(job.ID, data.JobRunning, "", "")`
   - `runner.RunThread(jobCtx, threadID, input)`
   - `store.UpdateJob(job.ID, data.JobCompleted/Failed, result.Response, err.Error())`
7. Return `{"job_id": job.ID.String()}`.

---

### 5. `internal/command/agent_thread.go` — Make `agent_chat` async

**`AgentChatCommand`** gains `store *data.Data` and `sched *scheduler.Scheduler`:

```go
// Before
type AgentChatCommand struct {
    runner *agent.Runner
}

// After
type AgentChatCommand struct {
    store  *data.Data
    runner *agent.Runner
    sched  *scheduler.Scheduler
}
```

`Execute` new logic (same pattern, no workspace context):

1. Parse `thread_id` and `input`.
2. Eagerly load via `store.GetAgentThread(threadID)`.
3. Create job: `store.CreateJob("agent_thread_chat", ksuid.KSUID{}, ksuid.KSUID{}, input)`.
4. Set `job.ThreadID = threadID`.
5. Attach cancellable context.
6. Schedule closure calling `runner.RunAgentThread(jobCtx, threadID, input)`.
7. Return `{"job_id": job.ID.String()}`.

---

### 6. `internal/command/command.go` — Wire up new fields in `InitCommands`

Three command instantiations change:

```go
// Before
CmdRunAgent: {cmd: &RunAgentCommand{runner: runner}},
CmdChat:     {cmd: &ChatCommand{runner: runner}},
CmdAgentChat:{cmd: &AgentChatCommand{runner: runner}},

// After
CmdRunAgent: {cmd: &RunAgentCommand{store: store, runner: runner, sched: sched}},
CmdChat:     {cmd: &ChatCommand{store: store, runner: runner, sched: sched}},
CmdAgentChat:{cmd: &AgentChatCommand{store: store, runner: runner, sched: sched}},
```

No other changes to `InitCommands`.

---

### 7. `pkg/client/client.go` — Return job IDs

Three methods change their return value from the raw response to a job ID:

```go
// Before
func (c *Client) RunAgent(agentID, input string) (string, error)
func (c *Client) Chat(threadID, message string) (string, error)
func (c *Client) AgentChat(threadID, message string) (string, error)

// After — all return the job_id string
func (c *Client) RunAgent(agentID, input string) (string, error)
func (c *Client) Chat(threadID, message string) (string, error)
func (c *Client) AgentChat(threadID, message string) (string, error)
```

The signatures are identical; only the parsing changes. Each method now parses `{"job_id": "..."}` instead of returning the raw string.

---

## Error Behaviour

Eager validation (before hitting the scheduler) means bad inputs still fail synchronously, with the same errors as today:

| Scenario | Behaviour |
|----------|-----------|
| `run_agent` with invalid/unknown agent_id | Immediate error response — no job created |
| `chat` with invalid/unknown thread_id | Immediate error response — no job created |
| `agent_chat` with invalid/unknown thread_id | Immediate error response — no job created |
| LLM call fails inside the async job | Job transitions to `failed`; error in `job_result` |
| `cancel_job` while job is running | Context cancelled; job transitions to `failed` |

---

## Tests to Update

### `internal/command/command_test.go`

- `RunAgent` tests currently pass `nil` runner and are param-validation-only — they remain valid. The nil `sched` in `InitCommands(store, nil, nil, nil)` is still safe because nil scheduler is never reached for param-only error tests.
- Add: `TestChatCommand_ReturnsJobID` — verify the response parses as `{"job_id":"..."}` when thread and runner are wired up (needs a fakeRunner or real runner with `t.TempDir()`).
- Add: `TestAgentChatCommand_ReturnsJobID` — same for agent_chat.
- Add: `TestRunAgentCommand_ReturnsJobID` — same for run_agent.

### `internal/agent/runner_test.go`

No changes — `Run`, `RunThread`, `RunAgentThread` are unchanged at the runner level.

### `cmd/integration/main.go`

- Update `testAgents` section: `run_agent` now returns a job ID; poll `job_status`/`job_result` for the response.
- Add `testThreads` section: call `chat`, poll result, verify thread history grows.
- Add to `testAgentThreads` section: call `agent_chat`, poll result.

---

## What Does NOT Change

- `runner.Run`, `runner.RunThread`, `runner.RunAgentThread` — unchanged.
- The scheduler, job lifecycle, `job_status`, `job_result`, `cancel_job` — unchanged.
- `run_pipeline` — already async, no changes needed.
- Validation logic — error messages remain the same; they just fire before the job is created.
