// Package client provides a Go client for the Slate agent orchestration server.
//
// The wire protocol is newline-delimited JSON:
//
//   Request:  {"cmd":"...", "params":{...}}\n
//   Response: {"ok":true,"data":{...}}\n  or  {"ok":false,"error":"..."}\n
//
// # Basic usage
//
//	c, err := client.Dial("localhost:4242")
//	if err != nil { ... }
//	defer c.Close()
//
//	err = c.AddWorkspace("my-workspace")
//
// # External agent registration
//
// An external process can register itself as an agent using AgentSession:
//
//	sess, err := client.DialAgentSession("localhost:4242", catalogID, "my-agent", "You are helpful.")
//	if err != nil { ... }
//	sess.Run(ctx, func(ctx context.Context, input string) string {
//	    return "hello from external agent: " + input
//	})
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
)

// Client is a connection to a Slate server. Methods on Client are safe for
// concurrent use — each call is protected by an internal mutex.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
}

// Dial connects to the Slate server at addr and returns a ready Client.
func Dial(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// send writes a JSON command and reads the server's JSON response.
// It returns the raw "data" field on success, or an error if ok=false.
func (c *Client) send(cmd string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req, err := json.Marshal(map[string]interface{}{"cmd": cmd, "params": params})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if _, err := c.conn.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	resp, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")

	var envelope struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &envelope); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("%s", envelope.Error)
	}
	return envelope.Data, nil
}

// ---- Workspace commands ----

// AddWorkspace creates a workspace with the given name.
func (c *Client) AddWorkspace(name string) error {
	_, err := c.send("add_workspace", map[string]string{"name": name})
	return err
}

// DelWorkspace removes the named workspace.
func (c *Client) DelWorkspace(name string) error {
	_, err := c.send("del_workspace", map[string]string{"name": name})
	return err
}

// ---- Catalog commands ----

// AddCatalog creates a catalog with the given name.
func (c *Client) AddCatalog(name string) error {
	_, err := c.send("add_catalog", map[string]string{"name": name})
	return err
}

// DelCatalog removes the named catalog.
func (c *Client) DelCatalog(name string) error {
	_, err := c.send("del_catalog", map[string]string{"name": name})
	return err
}

// ListCatalogs returns all catalogs.
func (c *Client) ListCatalogs() ([]CatalogInfo, error) {
	data, err := c.send("ls_catalogs", map[string]string{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Catalogs []CatalogInfo `json:"catalogs"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Catalogs, nil
}

// ---- Agent commands ----

// AddAgent creates an agent in the given catalog. Returns the new agent's ID.
func (c *Client) AddAgent(catalogID, name string) (AgentInfo, error) {
	data, err := c.send("add_agent", map[string]string{"catalog_id": catalogID, "name": name})
	if err != nil {
		return AgentInfo{}, err
	}
	var info AgentInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return AgentInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// DelAgent removes an agent by ID.
func (c *Client) DelAgent(agentID string) error {
	_, err := c.send("del_agent", map[string]string{"agent_id": agentID})
	return err
}

// SetAgentInstructions sets the system prompt for an agent.
func (c *Client) SetAgentInstructions(agentID, instructions string) error {
	_, err := c.send("set_agent_instructions", map[string]string{"agent_id": agentID, "instructions": instructions})
	return err
}

// SetAgentModel sets the LLM model for an agent.
func (c *Client) SetAgentModel(agentID, model string) error {
	_, err := c.send("set_agent_model", map[string]string{"agent_id": agentID, "model": model})
	return err
}

// RunAgent starts an async single-turn run against an agent and returns the job ID.
func (c *Client) RunAgent(agentID, input string) (string, error) {
	data, err := c.send("run_agent", map[string]string{"agent_id": agentID, "input": input})
	if err != nil {
		return "", err
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return out.JobID, nil
}

// ---- Tool commands ----

// AddTool attaches a tool to an agent.
func (c *Client) AddTool(agentID, toolName string) error {
	_, err := c.send("add_tool", map[string]string{"agent_id": agentID, "tool": toolName})
	return err
}

// RemoveTool detaches a tool from an agent.
func (c *Client) RemoveTool(agentID, toolName string) error {
	_, err := c.send("remove_tool", map[string]string{"agent_id": agentID, "tool": toolName})
	return err
}

// ListTools returns the tools attached to an agent.
func (c *Client) ListTools(agentID string) ([]string, error) {
	data, err := c.send("ls_tools", map[string]string{"agent_id": agentID})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Tools, nil
}

// ---- Thread commands ----

// NewThread creates a thread inside a workspace. Returns the new thread's ID.
func (c *Client) NewThread(workspaceID, name string) (ThreadInfo, error) {
	data, err := c.send("new_thread", map[string]string{"workspace_id": workspaceID, "name": name})
	if err != nil {
		return ThreadInfo{}, err
	}
	var info ThreadInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ThreadInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// Chat sends a message to a thread and returns the job ID.
func (c *Client) Chat(threadID, message string) (string, error) {
	data, err := c.send("chat", map[string]string{"thread_id": threadID, "message": message})
	if err != nil {
		return "", err
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return out.JobID, nil
}

// ListThreads returns all threads in a workspace.
func (c *Client) ListThreads(workspaceID string) ([]ThreadInfo, error) {
	data, err := c.send("ls_threads", map[string]string{"workspace_id": workspaceID})
	if err != nil {
		return nil, err
	}
	var out struct {
		Threads []ThreadInfo `json:"threads"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Threads, nil
}

// ---- Agent thread commands ----

// NewAgentThread creates a thread bound to a specific agent. Returns the new thread's ID.
func (c *Client) NewAgentThread(agentID, name string) (AgentThreadInfo, error) {
	data, err := c.send("new_agent_thread", map[string]string{"agent_id": agentID, "name": name})
	if err != nil {
		return AgentThreadInfo{}, err
	}
	var info AgentThreadInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return AgentThreadInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// AgentChat sends a message to an agent thread and returns the job ID.
func (c *Client) AgentChat(threadID, message string) (string, error) {
	data, err := c.send("agent_chat", map[string]string{"thread_id": threadID, "message": message})
	if err != nil {
		return "", err
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return out.JobID, nil
}

// ListAgentThreads returns all threads for the given agent.
func (c *Client) ListAgentThreads(agentID string) ([]AgentThreadInfo, error) {
	data, err := c.send("ls_agent_threads", map[string]string{"agent_id": agentID})
	if err != nil {
		return nil, err
	}
	var out struct {
		Threads []AgentThreadInfo `json:"threads"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Threads, nil
}

// AgentThreadHistory returns the raw JSON message history for an agent thread.
func (c *Client) AgentThreadHistory(threadID string) (json.RawMessage, error) {
	return c.send("agent_thread_history", map[string]string{"thread_id": threadID})
}

// ---- Workspace routing commands ----

// SetWorkspaceCatalog assigns a catalog to a workspace.
func (c *Client) SetWorkspaceCatalog(workspaceID, catalogID string) error {
	_, err := c.send("set_workspace_catalog", map[string]string{"workspace_id": workspaceID, "catalog_id": catalogID})
	return err
}

// SetWorkspaceRouter designates an agent as the workspace's router.
func (c *Client) SetWorkspaceRouter(workspaceID, agentID string) error {
	_, err := c.send("set_workspace_router", map[string]string{"workspace_id": workspaceID, "agent_id": agentID})
	return err
}

// ---- Pipeline commands ----

// CreatePipeline creates a named pipeline in a workspace. Returns the pipeline ID.
func (c *Client) CreatePipeline(workspaceID, name string) (PipelineInfo, error) {
	data, err := c.send("create_pipeline", map[string]string{"workspace_id": workspaceID, "name": name})
	if err != nil {
		return PipelineInfo{}, err
	}
	var info PipelineInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return PipelineInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// AddPipelineStep appends an agent step to a pipeline. mode is "sequential" or "parallel".
func (c *Client) AddPipelineStep(pipelineID, agentID, mode string) error {
	_, err := c.send("add_pipeline_step", map[string]string{"pipeline_id": pipelineID, "agent_id": agentID, "mode": mode})
	return err
}

// RunPipeline starts an async pipeline job and returns the job ID.
func (c *Client) RunPipeline(pipelineID, input string) (string, error) {
	data, err := c.send("run_pipeline", map[string]string{"pipeline_id": pipelineID, "input": input})
	if err != nil {
		return "", err
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return out.JobID, nil
}

// ---- Job commands ----

// JobStatus returns the current status of a job.
func (c *Client) JobStatus(jobID string) (JobStatus, error) {
	data, err := c.send("job_status", map[string]string{"job_id": jobID})
	if err != nil {
		return JobStatus{}, err
	}
	var status JobStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return JobStatus{}, fmt.Errorf("parsing response: %w", err)
	}
	return status, nil
}

// JobResult returns the result of a completed job.
func (c *Client) JobResult(jobID string) (JobResult, error) {
	data, err := c.send("job_result", map[string]string{"job_id": jobID})
	if err != nil {
		return JobResult{}, err
	}
	var result JobResult
	if err := json.Unmarshal(data, &result); err != nil {
		return JobResult{}, fmt.Errorf("parsing response: %w", err)
	}
	return result, nil
}

// WaitJob blocks until the job reaches a terminal state and returns the result.
func (c *Client) WaitJob(jobID string) (JobResult, error) {
	data, err := c.send("wait_job", map[string]string{"job_id": jobID})
	if err != nil {
		return JobResult{}, err
	}
	var result JobResult
	if err := json.Unmarshal(data, &result); err != nil {
		return JobResult{}, fmt.Errorf("parsing response: %w", err)
	}
	return result, nil
}

// ListJobs returns all jobs, optionally filtered by workspaceID (pass "" for all).
func (c *Client) ListJobs(workspaceID string) ([]JobInfo, error) {
	data, err := c.send("ls_jobs", map[string]string{"workspace_id": workspaceID})
	if err != nil {
		return nil, err
	}
	var jobs []JobInfo
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return jobs, nil
}

// CancelJob cancels a running job.
func (c *Client) CancelJob(jobID string) error {
	_, err := c.send("cancel_job", map[string]string{"job_id": jobID})
	return err
}

// ---- Management commands ----

// Health checks the server health.
func (c *Client) Health() error {
	_, err := c.send("health", map[string]string{})
	return err
}

// SystemMetrics returns server metrics.
func (c *Client) SystemMetrics() (json.RawMessage, error) {
	return c.send("system_metrics", map[string]string{})
}

// SystemStats returns combined server stats.
func (c *Client) SystemStats() (json.RawMessage, error) {
	return c.send("system_stats", map[string]string{})
}

// ---- AgentSession — external agent registration ----

// AgentSession represents a connection that has been registered as an external agent.
type AgentSession struct {
	agentID string
	conn    net.Conn
	reader  *bufio.Reader
}

// DialAgentSession connects to addr, registers as an external agent in the given
// catalog, and returns an AgentSession ready to receive and handle run requests.
func DialAgentSession(addr, catalogID, name, instructions string) (*AgentSession, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	reader := bufio.NewReader(conn)

	// Send registration command as JSON.
	req, _ := json.Marshal(map[string]interface{}{
		"cmd": "register_agent",
		"params": map[string]string{
			"catalog_id":   catalogID,
			"name":         name,
			"instructions": instructions,
		},
	})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending register_agent: %w", err)
	}

	// Read the {"ok":true,"data":{"agent_id":"..."}} response.
	resp, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading registration response: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")

	var envelope struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &envelope); err != nil {
		conn.Close()
		return nil, fmt.Errorf("parsing registration response: %w", err)
	}
	if !envelope.OK {
		conn.Close()
		return nil, fmt.Errorf("registration failed: %s", envelope.Error)
	}

	var out struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(envelope.Data, &out); err != nil {
		conn.Close()
		return nil, fmt.Errorf("parsing agent_id: %w", err)
	}

	return &AgentSession{
		agentID: out.AgentID,
		conn:    conn,
		reader:  reader,
	}, nil
}

// AgentID returns the server-assigned ID for this agent.
func (s *AgentSession) AgentID() string {
	return s.agentID
}

// Close closes the underlying connection.
func (s *AgentSession) Close() error {
	return s.conn.Close()
}

// HandlerFunc is called by Run for each incoming request from the orchestrator.
type HandlerFunc func(ctx context.Context, input string) string

// Run enters the agent receive loop, calling handler for each request from the
// server. It returns when ctx is cancelled or the server closes the connection.
func (s *AgentSession) Run(ctx context.Context, handler HandlerFunc) error {
	type serverRequest struct {
		RunID string `json:"run_id"`
		Input string `json:"input"`
	}
	type clientResponse struct {
		RunID    string `json:"run_id"`
		Response string `json:"response"`
	}

	readErr := make(chan error, 1)

	go func() {
		for {
			line, err := s.reader.ReadString('\n')
			if err != nil {
				readErr <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")

			var req serverRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}

			response := handler(ctx, req.Input)

			resp := clientResponse{RunID: req.RunID, Response: response}
			data, _ := json.Marshal(resp)
			if _, err := s.conn.Write(append(data, '\n')); err != nil {
				readErr <- err
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-readErr:
		return err
	}
}
