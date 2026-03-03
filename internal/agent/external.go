package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/segmentio/ksuid"
)

// agentRequest is sent from the server to an external agent over the wire.
type agentRequest struct {
	RunID string `json:"run_id"`
	Input string `json:"input"`
}

// agentResponse is received from an external agent.
type agentResponse struct {
	RunID    string `json:"run_id"`
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// AgentConn manages the TCP connection to a single registered external agent.
type AgentConn struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	done   chan struct{}
}

func newAgentConn(conn net.Conn) *AgentConn {
	return &AgentConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
		done:   make(chan struct{}),
	}
}

// Run sends input to the external agent and waits for a response.
// Concurrent calls are serialized via the internal mutex.
func (ac *AgentConn) Run(ctx context.Context, input string) (string, error) {
	runID := ksuid.New().String()

	ac.mu.Lock()
	defer ac.mu.Unlock()

	req := agentRequest{RunID: runID, Input: input}
	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}
	if _, err = ac.conn.Write(append(data, '\n')); err != nil {
		ac.markDone()
		return "", fmt.Errorf("sending request: %w", err)
	}

	line, err := ac.reader.ReadString('\n')
	if err != nil {
		ac.markDone()
		return "", fmt.Errorf("reading response: %w", err)
	}

	var resp agentResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("agent error: %s", resp.Error)
	}
	return resp.Response, nil
}

// markDone closes the done channel exactly once.
func (ac *AgentConn) markDone() {
	select {
	case <-ac.done:
	default:
		close(ac.done)
	}
}

// Done returns a channel that is closed when the agent connection becomes inactive.
func (ac *AgentConn) Done() <-chan struct{} {
	return ac.done
}

// ExternalAgentRegistry tracks all currently connected external agent processes.
type ExternalAgentRegistry struct {
	mu     sync.RWMutex
	agents map[ksuid.KSUID]*AgentConn
}

// NewExternalAgentRegistry creates a new, empty registry.
func NewExternalAgentRegistry() *ExternalAgentRegistry {
	return &ExternalAgentRegistry{
		agents: make(map[ksuid.KSUID]*AgentConn),
	}
}

// Register creates an AgentConn for conn and associates it with id.
func (r *ExternalAgentRegistry) Register(id ksuid.KSUID, conn net.Conn) *AgentConn {
	ac := newAgentConn(conn)
	r.mu.Lock()
	r.agents[id] = ac
	r.mu.Unlock()
	return ac
}

// Unregister removes the agent connection for id.
func (r *ExternalAgentRegistry) Unregister(id ksuid.KSUID) {
	r.mu.Lock()
	delete(r.agents, id)
	r.mu.Unlock()
}

// Get returns the AgentConn for id, if registered.
func (r *ExternalAgentRegistry) Get(id ksuid.KSUID) (*AgentConn, bool) {
	r.mu.RLock()
	ac, ok := r.agents[id]
	r.mu.RUnlock()
	return ac, ok
}
