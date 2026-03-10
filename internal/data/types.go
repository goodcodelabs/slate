package data

import (
	"context"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/llm"
)

type Data struct {
	Workspaces map[ksuid.KSUID]*Workspace `msgpack:"workspaces"`
	Catalogs   map[ksuid.KSUID]*Catalog   `msgpack:"catalogs"`
	Threads    map[ksuid.KSUID]*Thread    `msgpack:"-"`
	Pipelines  map[ksuid.KSUID]*Pipeline  `msgpack:"-"`
	Jobs       map[ksuid.KSUID]*Job       `msgpack:"-"`

	mu           sync.RWMutex             `msgpack:"-"`
	agentIndex   map[ksuid.KSUID]*Agent   `msgpack:"-"` // agentID → agent (O(1) lookup)
	agentCatalog map[ksuid.KSUID]*Catalog `msgpack:"-"` // agentID → owning catalog
	store        Store                    `msgpack:"-"`
}

type WorkspaceConfig struct {
	RouterAgentID ksuid.KSUID `msgpack:"router_agent_id"`
}

type Workspace struct {
	ID        ksuid.KSUID      `msgpack:"id"`
	Name      string           `msgpack:"name"`
	CatalogID ksuid.KSUID      `msgpack:"catalog_id"`
	Config    *WorkspaceConfig `msgpack:"config"`
}

type ThreadState string

const (
	ThreadActive    ThreadState = "active"
	ThreadCompleted ThreadState = "completed"
	ThreadError     ThreadState = "error"
)

// Thread is a persistent multi-turn conversation. If AgentID is non-zero the
// thread routes directly to that agent; otherwise routing goes through the
// workspace's router agent.
type Thread struct {
	ID          ksuid.KSUID   `msgpack:"id"`
	WorkspaceID ksuid.KSUID   `msgpack:"workspace_id"`
	AgentID     ksuid.KSUID   `msgpack:"agent_id"` // non-zero = agent-direct thread
	Name        string        `msgpack:"name"`
	State       ThreadState   `msgpack:"state"`
	CreatedAt   time.Time     `msgpack:"created_at"`
	UpdatedAt   time.Time     `msgpack:"updated_at"`

	// Messages are loaded from the per-thread log, not serialized in the snapshot.
	Messages []llm.Message `msgpack:"-"`
}

type Agent struct {
	ID           ksuid.KSUID       `msgpack:"id"`
	Name         string            `msgpack:"name"`
	Instructions string            `msgpack:"instructions"`
	Model        string            `msgpack:"model"`
	MaxTokens    int               `msgpack:"max_tokens"`
	Temperature  float64           `msgpack:"temperature"`
	Tools        []string          `msgpack:"tools"`
	Metadata     map[string]string `msgpack:"metadata"`
	External     bool              `msgpack:"external"`
}

type Catalog struct {
	ID   ksuid.KSUID `msgpack:"id"`
	Name string      `msgpack:"name"`

	Agents []*Agent `msgpack:"agents"`
}

// StepMode controls how a pipeline step executes relative to its neighbours.
type StepMode string

const (
	StepModeSequential StepMode = "sequential"
	StepModeParallel   StepMode = "parallel"
)

// PipelineStep is one agent invocation within a Pipeline.
type PipelineStep struct {
	AgentID ksuid.KSUID `msgpack:"agent_id"`
	Mode    StepMode    `msgpack:"mode"`
}

// Pipeline is an ordered sequence of agent steps attached to a workspace.
type Pipeline struct {
	ID          ksuid.KSUID    `msgpack:"id"`
	WorkspaceID ksuid.KSUID    `msgpack:"workspace_id"`
	Name        string         `msgpack:"name"`
	Steps       []PipelineStep `msgpack:"steps"`
}

// JobStatus tracks the lifecycle of an async job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

// Job represents an asynchronous unit of work (e.g., a pipeline run, a chat turn).
// Jobs are ephemeral — they live in memory and are not reloaded after restart.
type Job struct {
	ID          ksuid.KSUID        `msgpack:"id"`
	Type        string             `msgpack:"type"`
	WorkspaceID ksuid.KSUID        `msgpack:"workspace_id"`
	PipelineID  ksuid.KSUID        `msgpack:"pipeline_id"`
	ThreadID    ksuid.KSUID        `msgpack:"thread_id"`
	Input       string             `msgpack:"input"`
	Status      JobStatus          `msgpack:"status"`
	Result      string             `msgpack:"result"`
	Error       string             `msgpack:"error"`
	CreatedAt   time.Time          `msgpack:"created_at"`
	StartedAt   time.Time          `msgpack:"started_at"`
	CompletedAt time.Time          `msgpack:"completed_at"`

	// CancelFunc cancels the job's context. Not persisted.
	CancelFunc context.CancelFunc `msgpack:"-"`
}

// AgentThread is retained for backward-compatible loading of pre-migration
// snapshot files. New agent-direct threads are stored as Thread with AgentID set.
type AgentThread struct {
	ID        ksuid.KSUID   `msgpack:"id"`
	AgentID   ksuid.KSUID   `msgpack:"agent_id"`
	Name      string        `msgpack:"name"`
	State     ThreadState   `msgpack:"state"`
	CreatedAt time.Time     `msgpack:"created_at"`
	UpdatedAt time.Time     `msgpack:"updated_at"`
	Messages  []llm.Message `msgpack:"-"`
}
