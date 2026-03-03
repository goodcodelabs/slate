imple# Slate: Unit Test Plan

## Conventions

- Test files live next to the package they test: `foo_test.go` in the same directory.
- Use table-driven tests (`[]struct{ name, input, want }`) wherever multiple cases share the same shape.
- Use `t.TempDir()` for any test that touches the filesystem.
- Where a test needs an interface implementation it doesn't own, create a minimal hand-rolled fake in `_test.go` — avoid heavy mocking frameworks.
- Tests that make real network or LLM calls are integration tests; they live in `cmd/integration/` and are excluded from `make test`.

---

## internal/parser

**File:** `internal/parser/parser_test.go`

| Test | What to verify |
|------|----------------|
| `TestParseRequest_SimpleCommand` | Single-word command with no params → `Command="health"`, `Params=[]` |
| `TestParseRequest_CommandWithParams` | `"add_workspace foo"` → `Command="add_workspace"`, `Params=["foo"]` |
| `TestParseRequest_MultipleParams` | `"chat threadID hello world"` → params joined correctly |
| `TestParseRequest_CaseNormalisation` | `"HEALTH"` is lowercased to `"health"` |
| `TestParseRequest_LeadingTrailingWhitespace` | `" health \n"` parses cleanly |
| `TestParseRequest_EmptyInput` | Empty string returns error |
| `TestParseRequest_OnlyWhitespace` | Whitespace-only string returns error |

---

## internal/metrics

**File:** `internal/metrics/metrics_test.go`

| Test | What to verify |
|------|----------------|
| `TestNew_InitialState` | All counters zero; `LLMLatencyMin = -1` sentinel |
| `TestRecordLLMCall_AccumulatesTokens` | `InputTokens` and `OutputTokens` sum across multiple calls |
| `TestRecordLLMCall_LatencyStats` | `Min`, `Max`, `Avg` correct after several calls with known latencies |
| `TestRecordLLMCall_FirstCall_SetsMin` | First call initialises min from -1 |
| `TestRecordToolCall_Increments` | `ToolCalls` increments |
| `TestRecordError_Increments` | `Errors` increments |
| `TestConnections_IncrDecr` | `ActiveConnections` follows paired incr/decr |
| `TestSnapshot_Consistency` | Snapshot taken between writes reflects state at that moment |
| `TestMarshalJSON_ValidJSON` | `json.Unmarshal` of `MarshalJSON()` output succeeds |
| `TestRecordLLMCall_Concurrent` | 100 goroutines recording simultaneously; final count is exact |

---

## internal/events

**File:** `internal/events/events_test.go`

| Test | What to verify |
|------|----------------|
| `TestNewLogger_CreatesDirectory` | `events/` directory created under `dataDir` |
| `TestAppend_WritesJSONLine` | Line in file is valid JSON matching the event |
| `TestAppend_PerWorkspaceFiles` | Events for two workspaces go to two separate files |
| `TestAppend_EmptyWorkspaceID_NoOp` | Missing `WorkspaceID` writes nothing; no error |
| `TestAppend_TimestampAutoFilled` | Zero-time `Timestamp` is set before writing |
| `TestAppend_AppendsMultipleLines` | Second write does not overwrite first |
| `TestAppend_Concurrent` | 50 goroutines appending to same workspace; all lines present, none corrupt |

---

## internal/scheduler

**File:** `internal/scheduler/scheduler_test.go`

| Test | What to verify |
|------|----------------|
| `TestScheduler_RunsActivity` | Scheduled job executes |
| `TestScheduler_MultipleWorkers` | N activities complete, each on a worker goroutine |
| `TestScheduler_StopDrains` | After `Stop()`, all already-queued activities complete |
| `TestScheduler_QueueDepth` | `QueueDepth()` reflects pending count before workers consume |
| `TestScheduler_NilActivity` | Scheduling `nil` does not panic |
| `TestScheduler_Concurrency` | 64 activities scheduled; all complete without data race (run with `-race`) |

---

## internal/data

### data_test.go — in-memory operations (no filesystem, nil store)

| Test | What to verify |
|------|----------------|
| `TestAddWorkspace_Success` | Workspace created, available in map |
| `TestAddWorkspace_Duplicate` | Second `AddWorkspace("x")` returns error |
| `TestRemoveWorkspace_Success` | Workspace removed from map |
| `TestRemoveWorkspace_Missing` | Returns error |
| `TestAddCatalog_Success` | Catalog created |
| `TestAddCatalog_Duplicate` | Returns error |
| `TestRemoveCatalog_Success` | Catalog removed |
| `TestListCatalogs` | Returns all catalogs |
| `TestAddAgent_Success` | Agent added to catalog |
| `TestAddAgent_UnknownCatalog` | Returns error |
| `TestRegisterExternalAgent_SetsExternalFlag` | `agent.External == true` |
| `TestFindAgent_Found` | Returns agent and catalog |
| `TestFindAgent_NotFound` | Returns error |
| `TestSetAgentInstructions` | Instructions updated |
| `TestSetAgentModel` | Model updated |
| `TestAddAgentTool_Success` | Tool appended |
| `TestAddAgentTool_Duplicate` | Returns error |
| `TestRemoveAgentTool_Success` | Tool removed |
| `TestRemoveAgentTool_Missing` | Returns error |
| `TestNewThread_Success` | Thread created with correct workspace/agent IDs |
| `TestNewThread_UnknownWorkspace` | Returns error |
| `TestNewThread_UnknownAgent` | Returns error |
| `TestGetThread_Found` | Returns thread |
| `TestGetThread_Missing` | Returns error |
| `TestDeleteThread_Success` | Thread removed |
| `TestListThreads_FiltersByWorkspace` | Only threads for the given workspace returned |
| `TestAppendMessage` | Message appended to thread's slice |
| `TestGetWorkspace_Found` | Returns workspace |
| `TestGetWorkspace_Missing` | Returns error |
| `TestSetWorkspaceCatalog_Success` | `CatalogID` field updated |
| `TestSetWorkspaceCatalog_UnknownWorkspace` | Returns error |
| `TestSetWorkspaceCatalog_UnknownCatalog` | Returns error |
| `TestSetWorkspaceRouter_Success` | `RouterAgentID` updated |
| `TestNewPipeline_Success` | Pipeline created under workspace |
| `TestNewPipeline_UnknownWorkspace` | Returns error |
| `TestAddPipelineStep_Sequential` | Step appended with correct mode |
| `TestAddPipelineStep_InvalidAgent` | Returns error |
| `TestCreateJob_FieldsPopulated` | ID, status=Pending, timestamps set |
| `TestUpdateJob_Running` | `StartedAt` set |
| `TestUpdateJob_Completed` | `CompletedAt`, `Result` set |
| `TestUpdateJob_Failed` | `CompletedAt`, `Error` set |
| `TestCancelJob_CallsCancelFunc` | `CancelFunc` invoked |
| `TestListJobs_AllJobs` | Zero KSUID filter returns all |
| `TestListJobs_FilterByWorkspace` | Filters correctly |

### wal_test.go

| Test | What to verify |
|------|----------------|
| `TestWAL_AppendAndReplay_Workspace` | Add + remove workspace operations replay correctly |
| `TestWAL_AppendAndReplay_Catalog` | Add + remove catalog replays correctly |
| `TestWAL_AppendAndReplay_Thread` | Add + remove thread replays correctly |
| `TestWAL_AppendAndReplay_Pipeline` | Add + remove pipeline replays correctly |
| `TestWAL_SequenceMonotonicallyIncreases` | Each append gets a higher sequence number |
| `TestWAL_Truncate_ClearsFile` | After truncate, replay applies nothing |
| `TestWAL_ReplayFindsMaxSequence` | After replay, `sequence` equals the highest entry |
| `TestWAL_CorruptLine_ReturnsError` | A malformed JSON line causes `Replay` to return error |
| `TestWAL_EmptyFile_ReplayNoOp` | No error, empty data unchanged |
| `TestWAL_Concurrent_Append` | 20 concurrent appends all visible after replay |

### filestore_test.go

| Test | What to verify |
|------|----------------|
| `TestNewFileStore_CreatesDirectories` | All snapshot subdirs created on first open |
| `TestSaveAndLoadWorkspace` | Saved workspace survives store close + reopen |
| `TestDeleteWorkspace_RemovesFile` | File gone after delete; reload doesn't contain workspace |
| `TestSaveAndLoadCatalog` | Catalog with agents survives round-trip |
| `TestSaveAndLoadThread` | Thread metadata persisted; messages stored in log |
| `TestAppendMessage_PersistsToLog` | Reloaded thread contains appended message |
| `TestAppendMessage_MultipleMessages` | All messages present in order after reload |
| `TestSaveAndLoadPipeline` | Pipeline with steps survives round-trip |
| `TestCheckpoint_TruncatesWAL` | WAL file size drops after checkpoint |
| `TestAtomicWrite_NoCrossContamination` | Simulated mid-write crash leaves original intact (write to temp, rename) |
| `TestLoad_WALReplayedAfterSnapshots` | Entity added by WAL but not snapshotted is present after load |

---

## internal/tools

### registry_test.go

| Test | What to verify |
|------|----------------|
| `TestRegistry_RegisterAndGet` | Registered tool retrievable by name |
| `TestRegistry_GetMissing` | `ok=false` for unknown tool |
| `TestRegistry_Names` | Returns all registered names |
| `TestRegistry_GetDefs_SkipsMissing` | Unknown names silently omitted from batch result |
| `TestRegistry_Execute_Success` | Tool executes and returns output |
| `TestRegistry_Execute_UnknownTool` | Returns error |
| `TestRegistry_Execute_ToolError` | Tool execution error propagated |

### builtin/http_test.go

| Test | What to verify |
|------|----------------|
| `TestHTTPFetch_GET` | GET to test server returns body and `200` status |
| `TestHTTPFetch_POST` | POST with body sends body; server receives it |
| `TestHTTPFetch_Headers` | Custom headers forwarded to server |
| `TestHTTPFetch_4xxResponse` | Non-2xx status still returned (not an error) |
| `TestHTTPFetch_ResponseTruncatedAt1MB` | Response > 1 MB truncated in output |
| `TestHTTPFetch_Timeout` | Request to slow server respects 30 s deadline |
| `TestHTTPFetch_ContextCancellation` | Cancelled context aborts request |
| `TestHTTPFetch_InvalidURL` | Returns error |
| `TestHTTPFetch_Definition` | `Definition().Name == "http_fetch"`, schema valid JSON |

### builtin/shell_test.go

| Test | What to verify |
|------|----------------|
| `TestShell_SimpleCommand` | `echo hello` → `output="hello\n"`, `exit_code=0` |
| `TestShell_ExitCode` | Non-zero exit code captured |
| `TestShell_Stderr` | stderr captured in output |
| `TestShell_Timeout_EnforcedDefault` | Long-running command killed at 30 s |
| `TestShell_Timeout_CustomSeconds` | Custom timeout respected (≤ 120 s) |
| `TestShell_Timeout_ClampedAt120` | Value > 120 clamped or errors |
| `TestShell_ContextCancellation` | Cancelled ctx kills process |
| `TestShell_Definition` | `Definition().Name == "shell"`, schema valid JSON |

### builtin/files_test.go

| Test | What to verify |
|------|----------------|
| `TestFile_Write_CreatesFile` | File written with correct content |
| `TestFile_Write_CreatesParentDirs` | Missing parent directories created |
| `TestFile_Read_ReturnsContent` | Previously written content returned |
| `TestFile_Read_Missing` | Returns error for non-existent file |
| `TestFile_Append_AddsContent` | Existing file has content appended |
| `TestFile_Append_CreatesFile` | Append to non-existent file creates it |
| `TestFile_PathTraversal_Rejected` | Path containing `..` returns error |
| `TestFile_UnknownAction` | Invalid action returns error |
| `TestFile_Definition` | `Definition().Name == "file"`, schema valid JSON |

---

## internal/agent

### external_test.go

Use `net.Pipe()` to create an in-process connection pair — no real TCP needed.

| Test | What to verify |
|------|----------------|
| `TestAgentConn_Run_Success` | JSON request sent; JSON response read; response string returned |
| `TestAgentConn_Run_AgentError` | Response with `error` field → error returned |
| `TestAgentConn_Run_ParseError` | Malformed JSON line → error returned |
| `TestAgentConn_Run_WriteError` | Closed connection on write → error, `Done()` closed |
| `TestAgentConn_Run_ReadError` | Closed connection on read → error, `Done()` closed |
| `TestAgentConn_Done_ClosedOnError` | `Done()` channel closed after I/O error |
| `TestAgentConn_Concurrent_Serialised` | Two goroutines call `Run` concurrently; both get correct responses without interleaving |
| `TestRegistry_RegisterGetUnregister` | Register → Get (found) → Unregister → Get (not found) |
| `TestRegistry_Concurrent` | 20 goroutines registering/getting/unregistering concurrently; no data race |

### runner_test.go

Use a fake `llm.Provider` that returns canned `CompletionResponse` values.

| Test | What to verify |
|------|----------------|
| `TestRun_SingleTurn` | Provider called once, response text returned |
| `TestRun_DefaultModelUsed` | Agent with empty model uses `defaultModel` |
| `TestRun_DefaultMaxTokensUsed` | Agent with `MaxTokens=0` uses `defaultMaxTokens` |
| `TestRun_SystemPromptSuffix` | `RunOptions.SystemPromptSuffix` appended to instructions |
| `TestRun_ToolCallLoop` | Provider returns `tool_use`, registry executes tool, second provider call returns text |
| `TestRun_ToolResult_IsError` | Registry error → `Content.IsError=true` in tool_result turn |
| `TestRun_NoTools_SkipsRegistry` | `stop_reason != tool_use` on first response → no registry call |
| `TestRun_TokensAccumulated` | Tokens from two iterations summed in `RunResult` |
| `TestRun_ProviderError_RecordsMetric` | Provider error → `metrics.Errors` incremented |
| `TestRun_ContextCancellation` | Cancelled context propagated to provider |
| `TestRun_ExternalAgent_Dispatched` | `agent.External=true` → `ExternalAgentRegistry.Run` called, LLM not called |
| `TestRun_ExternalAgent_NotConnected` | External agent not in registry → error |
| `TestRun_ExternalAgent_RegistryNil` | `externalAgents==nil` → error |
| `TestRunThread_AppendsMessages` | User message + assistant message appended to thread |
| `TestRunThread_UsesExistingHistory` | Existing thread messages included in provider call |
| `TestRunThread_EmitsEvents` | `EventAgentRunStarted` and `EventAgentRunCompleted` emitted |
| `TestRunThread_EmitsFailedEvent` | Provider error → `EventAgentRunFailed` emitted |
| `TestRunWithOptions_MetricsRecorded` | `RecordLLMCall` called with correct latency + tokens |

### pipeline_test.go

| Test | What to verify |
|------|----------------|
| `TestRunPipeline_SingleSequentialStep` | Single step receives input, returns output |
| `TestRunPipeline_ChainedSequential` | Step 2 input = step 1 output |
| `TestRunPipeline_ParallelSteps` | Both steps receive same input; outputs joined with `\n---\n` |
| `TestRunPipeline_MixedModes` | Sequential → parallel group → sequential produces correct chain |
| `TestRunPipeline_StepFailure` | Error in step → pipeline returns error, subsequent steps not run |
| `TestRunPipeline_EmptyPipeline` | No steps → returns original input |
| `TestRunPipeline_EmitsStartedEvent` | `EventPipelineStarted` emitted |
| `TestRunPipeline_EmitsCompletedEvent` | `EventPipelineCompleted` emitted on success |
| `TestRunPipeline_EmitsFailedEvent` | `EventPipelineFailed` emitted on error |
| `TestRunPipeline_ContextCancellation` | Cancelling ctx stops mid-pipeline |

### router_test.go

| Test | What to verify |
|------|----------------|
| `TestRunWorkspaceChat_NoRouter` | Workspace without `RouterAgentID` returns error |
| `TestRunWorkspaceChat_RouterCalled` | Provider called with router agent's instructions |
| `TestRunWorkspaceChat_CatalogListingInjected` | System prompt includes agent IDs and names from catalog |
| `TestRunWorkspaceChat_NoCatalog` | Workspace with no `CatalogID` still calls router (no crash) |
| `TestRunWorkspaceChat_LongDescriptionTruncated` | Instructions > 120 chars truncated with `"..."` |
| `TestRunWorkspaceChat_EmitsEvents` | Started/Completed events emitted with workspace ID |

### calltool_test.go

| Test | What to verify |
|------|----------------|
| `TestCallAgentTool_Definition` | Name is `"call_agent"`; schema has `agent_id` and `input` required |
| `TestCallAgentTool_Execute_Success` | Valid JSON → runner called; response JSON-marshaled |
| `TestCallAgentTool_Execute_MissingAgentID` | Missing `agent_id` → error |
| `TestCallAgentTool_Execute_InvalidAgentID` | Non-KSUID `agent_id` → error |
| `TestCallAgentTool_Execute_RunnerError` | Runner error → tool error |

---

## internal/command

Create a shared test helper `newTestStore(t)` that returns a `*data.Data` with `nil` store (in-memory only).

### command_test.go

| Test | What to verify |
|------|----------------|
| `TestInitCommands_AllKeysPresent` | Every `Cmd*` constant is a key in the returned map |

### workspace_test.go

| Test | What to verify |
|------|----------------|
| `TestAddWorkspaceCommand_Success` | Returns "ok" |
| `TestAddWorkspaceCommand_MissingParam` | Returns error |
| `TestDelWorkspaceCommand_Success` | Returns "ok" |
| `TestSetWorkspaceCatalogCommand_InvalidID` | Invalid KSUID param returns error |
| `TestSetWorkspaceCatalogCommand_Success` | Returns "ok" when IDs valid |
| `TestSetWorkspaceRouterCommand_InvalidID` | Invalid agent_id returns error |

### catalog_test.go

| Test | What to verify |
|------|----------------|
| `TestAddCatalogCommand_Success` | Returns "ok" |
| `TestRemoveCatalogCommand_Success` | Returns "ok" |
| `TestListCatalogsCommand_EmptyJSON` | Returns `{"catalogs":[]}` when no catalogs |
| `TestListCatalogsCommand_ContainsCreatedCatalog` | Catalog appears in response after add |

### agent_test.go

| Test | What to verify |
|------|----------------|
| `TestAddAgentCommand_Success` | Returns JSON with `id` and `name` |
| `TestAddAgentCommand_InvalidCatalogID` | Returns error |
| `TestAddAgentCommand_MissingParams` | Returns error |
| `TestSetAgentInstructionsCommand_MultiWordInstructions` | Words joined with space |
| `TestSetAgentModelCommand_Success` | Returns "ok" |
| `TestRunAgentCommand_DelegatesToRunner` | Fake runner invoked with correct agentID and input |

### thread_test.go

| Test | What to verify |
|------|----------------|
| `TestNewThreadCommand_DefaultName` | Auto-name generated when no name param |
| `TestNewThreadCommand_CustomName` | Multi-word name joined |
| `TestNewThreadCommand_InvalidWorkspaceID` | Returns error |
| `TestChatCommand_DelegatesToRunner` | `RunThread` called with correct threadID and input |
| `TestListThreadsCommand_ReturnsJSON` | Valid JSON with `threads` array |
| `TestThreadHistoryCommand_ReturnsMessages` | JSON with `messages` key |

### tools_test.go

| Test | What to verify |
|------|----------------|
| `TestAddToolCommand_Success` | Returns "ok" |
| `TestAddToolCommand_MissingParams` | Returns error |
| `TestRemoveToolCommand_Success` | Returns "ok" |
| `TestListToolsCommand_ReturnsJSON` | JSON with `tools` array; empty list not null |

### pipeline_test.go

| Test | What to verify |
|------|----------------|
| `TestCreatePipelineCommand_Success` | Returns `{"pipeline_id": "..."}` |
| `TestAddPipelineStepCommand_InvalidMode` | Mode not `sequential`/`parallel` → error |
| `TestAddPipelineStepCommand_Success` | Returns "ok" |

### jobs_test.go

| Test | What to verify |
|------|----------------|
| `TestJobStatusCommand_MissingParam` | Returns error |
| `TestJobStatusCommand_InvalidID` | Returns error |
| `TestJobStatusCommand_Found` | Returns JSON with `status` field |
| `TestJobResultCommand_Found` | Returns JSON with `result` and `error` fields |
| `TestListJobsCommand_AllJobs` | Returns JSON array |
| `TestListJobsCommand_FilteredByWorkspace` | Only matching workspace jobs returned |
| `TestCancelJobCommand_Success` | Returns "ok" |

### management_test.go

| Test | What to verify |
|------|----------------|
| `TestHealthCommand` | Returns "ok" |
| `TestSystemMetricsCommand_ValidJSON` | Output unmarshals |
| `TestSystemStatsCommand_ValidJSON` | Output unmarshals; has `jobs`, `scheduler_queue` keys |

---

## internal/connection

**File:** `internal/connection/connection_test.go`

Use `net.Pipe()` for an in-process connection; a stub `*data.Data`, a stub runner, and a stub scheduler.

| Test | What to verify |
|------|----------------|
| `TestHandleConnection_HealthCommand` | `"health\n"` → `"ok\n"` |
| `TestHandleConnection_UnknownCommand` | Unknown command → `"error|invalid_command\n"` |
| `TestHandleConnection_InvalidSyntax` | Empty request → `"error|invalid_syntax\n"` |
| `TestHandleConnection_QuitCloses` | `"quit\n"` → responds "ok", then connection closed |
| `TestHandleConnection_ErrorPropagation` | Command that errors → response prefixed `"error|"` |
| `TestHandleConnection_ContextCancellation` | Cancelling context causes loop to exit |
| `TestHandleConnection_MetricsIncrement` | Active connections incremented on start, decremented on end |
| `TestHandleConnection_RegisterAgent_NoRegistry` | `register_agent` with nil registry → `"error|..."` |
| `TestHandleConnection_RegisterAgent_MissingParams` | Fewer than 3 params → `"error|..."` |
| `TestRespond` | Writes message + newline to connection |

---

## pkg/client

**File:** `pkg/client/client_test.go`

Use `net.Pipe()` + a goroutine that echoes/stubs server responses.

| Test | What to verify |
|------|----------------|
| `TestClient_Send_Success` | Writes `"cmd arg\n"`, reads response |
| `TestClient_Send_ErrorPrefix` | Response starting `"error|foo"` → `err.Error() == "foo"` |
| `TestClient_AddWorkspace` | Sends `"add_workspace name\n"` |
| `TestClient_AddCatalog` | Sends `"add_catalog name\n"` |
| `TestClient_ListCatalogs_ParsesJSON` | JSON response parsed into `[]CatalogInfo` |
| `TestClient_AddAgent_ParsesJSON` | JSON response parsed into `AgentInfo` with ID |
| `TestClient_NewThread_ParsesJSON` | JSON response parsed into `ThreadInfo` |
| `TestClient_ListThreads_ParsesJSON` | JSON `{"threads":[...]}` parsed correctly |
| `TestClient_RunPipeline_ParsesJobID` | JSON `{"job_id":"..."}` parsed |
| `TestClient_JobStatus_ParsesJSON` | JSON status struct parsed |
| `TestClient_JobResult_ParsesJSON` | JSON result struct parsed |
| `TestClient_ListJobs_ParsesJSON` | JSON array parsed |
| `TestClient_ListJobs_NoFilter` | Sends `"ls_jobs\n"` without workspace param |
| `TestClient_Concurrent` | Two goroutines call `Health` concurrently; both get responses without interleaving |
| `TestDialAgentSession_ReceivesAgentID` | Sends `register_agent` command; response parsed to AgentID |
| `TestDialAgentSession_ErrorResponse` | Server returns `"error|..."` → error returned |
| `TestAgentSession_Run_Calls_Handler` | Server sends JSON request; handler called; JSON response written back |
| `TestAgentSession_Run_ContextCancel` | Cancelled ctx causes `Run` to return `context.Canceled` |
| `TestAgentSession_Run_ConnectionClosed` | Server closes pipe; `Run` returns error |

---

## Running Tests

```sh
# All unit tests
make test

# With race detector (recommended for CI)
go test -race ./...

# Single package
go test ./internal/data/...

# Integration tests (requires running server)
make integrate
```

## Suggested Implementation Order

1. `internal/parser` — no dependencies, fast wins
2. `internal/metrics` — pure in-memory logic
3. `internal/events` — filesystem only
4. `internal/scheduler` — goroutines + channels
5. `internal/data` — core domain logic (wal → filestore → data)
6. `internal/tools` — isolated tool behaviour
7. `internal/agent` — requires fakes for `llm.Provider` and `data.Data`
8. `internal/command` — thin layer over data + runner
9. `internal/connection` — requires `net.Pipe` harness
10. `pkg/client` — requires `net.Pipe` stub server
