// Package client provides a Go client for the Slate agent orchestration server.
//
// The wire protocol is line-oriented: each request is a space-separated list of
// tokens followed by a newline; the server replies with a single line. Error
// responses are prefixed with "error|".
//
// # Basic usage
//
//	c, err := client.Dial("localhost:6379")
//	if err != nil { ... }
//	defer c.Close()
//
//	wsID, err := c.AddWorkspace("my-workspace")
//
// # External agent registration
//
// An external process can register itself as an agent using AgentSession:
//
//	sess, err := client.DialAgentSession("localhost:6379", catalogID, "my-agent", "You are helpful.")
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

// send writes a command line and reads the server's single-line response.
// It returns an error if the response begins with "error|".
func (c *Client) send(args ...string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	line := strings.Join(args, " ") + "\n"
	if _, err := c.conn.Write([]byte(line)); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	resp, err := c.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")

	if strings.HasPrefix(resp, "error|") {
		return "", fmt.Errorf("%s", strings.TrimPrefix(resp, "error|"))
	}
	return resp, nil
}

// ---- Workspace commands ----

// AddWorkspace creates a workspace with the given name.
func (c *Client) AddWorkspace(name string) error {
	_, err := c.send("add_workspace", name)
	return err
}

// DelWorkspace removes the named workspace.
func (c *Client) DelWorkspace(name string) error {
	_, err := c.send("del_workspace", name)
	return err
}

// ---- Catalog commands ----

// AddCatalog creates a catalog with the given name.
func (c *Client) AddCatalog(name string) error {
	_, err := c.send("add_catalog", name)
	return err
}

// DelCatalog removes the named catalog.
func (c *Client) DelCatalog(name string) error {
	_, err := c.send("del_catalog", name)
	return err
}

// ListCatalogs returns all catalogs.
func (c *Client) ListCatalogs() ([]CatalogInfo, error) {
	resp, err := c.send("ls_catalogs")
	if err != nil {
		return nil, err
	}
	var out struct {
		Catalogs []CatalogInfo `json:"catalogs"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Catalogs, nil
}

// ---- Agent commands ----

// AddAgent creates an agent in the given catalog. Returns the new agent's ID.
func (c *Client) AddAgent(catalogID, name string) (AgentInfo, error) {
	resp, err := c.send("add_agent", catalogID, name)
	if err != nil {
		return AgentInfo{}, err
	}
	var info AgentInfo
	if err := json.Unmarshal([]byte(resp), &info); err != nil {
		return AgentInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// SetAgentInstructions sets the system prompt for an agent.
func (c *Client) SetAgentInstructions(agentID, instructions string) error {
	_, err := c.send("set_agent_instructions", agentID, instructions)
	return err
}

// SetAgentModel sets the LLM model for an agent.
func (c *Client) SetAgentModel(agentID, model string) error {
	_, err := c.send("set_agent_model", agentID, model)
	return err
}

// RunAgent executes a single-turn run against an agent.
func (c *Client) RunAgent(agentID, input string) (string, error) {
	return c.send("run_agent", agentID, input)
}

// ---- Tool commands ----

// AddTool attaches a tool to an agent.
func (c *Client) AddTool(agentID, toolName string) error {
	_, err := c.send("add_tool", agentID, toolName)
	return err
}

// RemoveTool detaches a tool from an agent.
func (c *Client) RemoveTool(agentID, toolName string) error {
	_, err := c.send("remove_tool", agentID, toolName)
	return err
}

// ListTools returns the tools attached to an agent.
func (c *Client) ListTools(agentID string) ([]string, error) {
	resp, err := c.send("ls_tools", agentID)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Tools, nil
}

// ---- Thread commands ----

// NewThread creates a thread inside a workspace for the given agent.
// Returns the new thread's ID.
func (c *Client) NewThread(workspaceID, agentID string, name string) (ThreadInfo, error) {
	args := []string{"new_thread", workspaceID, agentID}
	if name != "" {
		args = append(args, name)
	}
	resp, err := c.send(args...)
	if err != nil {
		return ThreadInfo{}, err
	}
	var info ThreadInfo
	if err := json.Unmarshal([]byte(resp), &info); err != nil {
		return ThreadInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// Chat sends a message to a thread and returns the assistant's reply.
func (c *Client) Chat(threadID, message string) (string, error) {
	return c.send("chat", threadID, message)
}

// ListThreads returns all threads in a workspace.
func (c *Client) ListThreads(workspaceID string) ([]ThreadInfo, error) {
	resp, err := c.send("ls_threads", workspaceID)
	if err != nil {
		return nil, err
	}
	var out struct {
		Threads []ThreadInfo `json:"threads"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return out.Threads, nil
}

// ---- Workspace routing commands ----

// SetWorkspaceCatalog assigns a catalog to a workspace.
func (c *Client) SetWorkspaceCatalog(workspaceID, catalogID string) error {
	_, err := c.send("set_workspace_catalog", workspaceID, catalogID)
	return err
}

// SetWorkspaceRouter designates an agent as the workspace's router.
func (c *Client) SetWorkspaceRouter(workspaceID, agentID string) error {
	_, err := c.send("set_workspace_router", workspaceID, agentID)
	return err
}

// WorkspaceChat routes a message through the workspace's router agent.
func (c *Client) WorkspaceChat(workspaceID, message string) (string, error) {
	return c.send("workspace_chat", workspaceID, message)
}

// ---- Pipeline commands ----

// CreatePipeline creates a named pipeline in a workspace. Returns the pipeline ID.
func (c *Client) CreatePipeline(workspaceID, name string) (PipelineInfo, error) {
	resp, err := c.send("create_pipeline", workspaceID, name)
	if err != nil {
		return PipelineInfo{}, err
	}
	var info PipelineInfo
	if err := json.Unmarshal([]byte(resp), &info); err != nil {
		return PipelineInfo{}, fmt.Errorf("parsing response: %w", err)
	}
	return info, nil
}

// AddPipelineStep appends an agent step to a pipeline.
// mode is "sequential" or "parallel".
func (c *Client) AddPipelineStep(pipelineID, agentID, mode string) error {
	_, err := c.send("add_pipeline_step", pipelineID, agentID, mode)
	return err
}

// RunPipeline starts an async pipeline job and returns the job ID.
func (c *Client) RunPipeline(pipelineID, input string) (string, error) {
	resp, err := c.send("run_pipeline", pipelineID, input)
	if err != nil {
		return "", err
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return out.JobID, nil
}

// ---- Job commands ----

// JobStatus returns the current status of a job.
func (c *Client) JobStatus(jobID string) (JobStatus, error) {
	resp, err := c.send("job_status", jobID)
	if err != nil {
		return JobStatus{}, err
	}
	var status JobStatus
	if err := json.Unmarshal([]byte(resp), &status); err != nil {
		return JobStatus{}, fmt.Errorf("parsing response: %w", err)
	}
	return status, nil
}

// JobResult returns the result of a completed job.
func (c *Client) JobResult(jobID string) (JobResult, error) {
	resp, err := c.send("job_result", jobID)
	if err != nil {
		return JobResult{}, err
	}
	var result JobResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return JobResult{}, fmt.Errorf("parsing response: %w", err)
	}
	return result, nil
}

// ListJobs returns all jobs, optionally filtered by workspaceID (pass "" for all).
func (c *Client) ListJobs(workspaceID string) ([]JobInfo, error) {
	args := []string{"ls_jobs"}
	if workspaceID != "" {
		args = append(args, workspaceID)
	}
	resp, err := c.send(args...)
	if err != nil {
		return nil, err
	}
	var jobs []JobInfo
	if err := json.Unmarshal([]byte(resp), &jobs); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return jobs, nil
}

// CancelJob cancels a running job.
func (c *Client) CancelJob(jobID string) error {
	_, err := c.send("cancel_job", jobID)
	return err
}

// ---- Management commands ----

// Health checks the server health.
func (c *Client) Health() (string, error) {
	return c.send("health")
}

// SystemMetrics returns server metrics as a raw JSON string.
func (c *Client) SystemMetrics() (string, error) {
	return c.send("system_metrics")
}

// SystemStats returns combined server stats as a raw JSON string.
func (c *Client) SystemStats() (string, error) {
	return c.send("system_stats")
}

// ---- AgentSession — external agent registration ----

// AgentSession represents a connection that has been registered as an external agent.
// The server will call back through this connection when the agent needs to run.
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

	// Send registration command.
	line := strings.Join([]string{"register_agent", catalogID, name, instructions}, " ") + "\n"
	if _, err := conn.Write([]byte(line)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending register_agent: %w", err)
	}

	// Read the {"agent_id": "..."} response.
	resp, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading registration response: %w", err)
	}
	resp = strings.TrimRight(resp, "\r\n")

	if strings.HasPrefix(resp, "error|") {
		conn.Close()
		return nil, fmt.Errorf("registration failed: %s", strings.TrimPrefix(resp, "error|"))
	}

	var out struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		conn.Close()
		return nil, fmt.Errorf("parsing registration response: %w", err)
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
// The return value is sent back as the agent's response.
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
				// Unexpected message format — skip.
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
