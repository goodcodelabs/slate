package client

// WorkspaceInfo summarizes a workspace.
type WorkspaceInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CatalogID     string `json:"catalog_id,omitempty"`
	RouterAgentID string `json:"router_agent_id,omitempty"`
}

// AgentInfo summarizes a registered agent.
type AgentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ThreadInfo summarizes a conversation thread.
type ThreadInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Messages  int    `json:"messages"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AgentThreadInfo summarizes a conversation thread bound to a specific agent.
type AgentThreadInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	Messages  int    `json:"messages"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CatalogInfo summarizes a catalog.
type CatalogInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// JobInfo summarizes an async job.
type JobInfo struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	WorkspaceID string `json:"workspace_id"`
	PipelineID  string `json:"pipeline_id,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// JobStatus holds the status fields of a job.
type JobStatus struct {
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

// JobResult holds the outcome of a completed job.
type JobResult struct {
	Status string `json:"status"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

// PipelineInfo summarizes a pipeline.
type PipelineInfo struct {
	ID string `json:"pipeline_id"`
}
