package client_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"slate/pkg/client"
)

// fakeServer is a minimal test server that handles Slate protocol commands.
type fakeServer struct {
	listener net.Listener
	handlers map[string]func(params []string) string
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &fakeServer{
		listener: ln,
		handlers: make(map[string]func(params []string) string),
	}

	// Default handlers.
	srv.handlers["health"] = func(_ []string) string { return "ok" }
	srv.handlers["add_workspace"] = func(_ []string) string { return "ok" }
	srv.handlers["del_workspace"] = func(_ []string) string { return "ok" }
	srv.handlers["add_catalog"] = func(_ []string) string { return "ok" }
	srv.handlers["del_catalog"] = func(_ []string) string { return "ok" }
	srv.handlers["ls_catalogs"] = func(_ []string) string {
		out, _ := json.Marshal(map[string]interface{}{
			"catalogs": []map[string]string{
				{"id": "cat1", "name": "test-cat"},
			},
		})
		return string(out)
	}

	go srv.serve()
	t.Cleanup(func() { ln.Close() })
	return srv
}

func (srv *fakeServer) Addr() string {
	return srv.listener.Addr().String()
}

func (srv *fakeServer) serve() {
	for {
		conn, err := srv.listener.Accept()
		if err != nil {
			return
		}
		go srv.handleConn(conn)
	}
}

func (srv *fakeServer) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToLower(parts[0])
		var params []string
		if len(parts) > 1 {
			params = strings.Split(parts[1], " ")
		}

		handler, ok := srv.handlers[cmd]
		var resp string
		if ok {
			resp = handler(params)
		} else {
			resp = fmt.Sprintf("error|unknown command: %s", cmd)
		}
		conn.Write([]byte(resp + "\n"))
	}
}

// ---- Client tests ----

func TestClient_Health(t *testing.T) {
	srv := newFakeServer(t)
	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	resp, err := c.Health()
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if resp != "ok" {
		t.Errorf("Health = %q, want %q", resp, "ok")
	}
}

func TestClient_AddWorkspace(t *testing.T) {
	srv := newFakeServer(t)
	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.AddWorkspace("test-ws"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
}

func TestClient_DelWorkspace(t *testing.T) {
	srv := newFakeServer(t)
	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.DelWorkspace("test-ws"); err != nil {
		t.Fatalf("DelWorkspace: %v", err)
	}
}

func TestClient_AddCatalog(t *testing.T) {
	srv := newFakeServer(t)
	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.AddCatalog("my-catalog"); err != nil {
		t.Fatalf("AddCatalog: %v", err)
	}
}

func TestClient_ListCatalogs(t *testing.T) {
	srv := newFakeServer(t)
	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	cats, err := c.ListCatalogs()
	if err != nil {
		t.Fatalf("ListCatalogs: %v", err)
	}
	if len(cats) != 1 {
		t.Errorf("expected 1 catalog, got %d", len(cats))
	}
	if cats[0].Name != "test-cat" {
		t.Errorf("catalog name = %q, want %q", cats[0].Name, "test-cat")
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	srv := newFakeServer(t)
	// Register an error-returning handler.
	srv.handlers["add_workspace"] = func(_ []string) string {
		return "error|workspace already exists"
	}

	c, err := client.Dial(srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	err = c.AddWorkspace("existing")
	if err == nil {
		t.Fatal("expected error from server error response, got nil")
	}
	if !strings.Contains(err.Error(), "workspace already exists") {
		t.Errorf("error = %q, want to contain 'workspace already exists'", err.Error())
	}
}

func TestClient_Dial_BadAddress(t *testing.T) {
	_, err := client.Dial("127.0.0.1:1") // port 1 should be refused
	if err == nil {
		t.Fatal("expected error dialing bad address, got nil")
	}
}

func TestAgentSession_Run(t *testing.T) {
	// Create a server that simulates the register_agent handshake and external agent protocol.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)

		// Read the register_agent command.
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		_ = line // consume register_agent line

		// Send agent_id response.
		resp, _ := json.Marshal(map[string]string{"agent_id": "test-agent-id-123"})
		conn.Write(append(resp, '\n'))

		// Send a run request to the agent.
		req, _ := json.Marshal(map[string]string{"run_id": "run1", "input": "hello"})
		conn.Write(append(req, '\n'))

		// Read response from agent session.
		respLine, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		var agentResp struct {
			RunID    string `json:"run_id"`
			Response string `json:"response"`
		}
		json.Unmarshal([]byte(strings.TrimRight(respLine, "\r\n")), &agentResp)
		if agentResp.Response != "echo: hello" {
			t.Errorf("agent response = %q, want %q", agentResp.Response, "echo: hello")
		}
	}()

	sess, err := client.DialAgentSession(ln.Addr().String(), "catalog-id", "test-agent", "instructions")
	if err != nil {
		t.Fatalf("DialAgentSession: %v", err)
	}
	defer sess.Close()

	if sess.AgentID() != "test-agent-id-123" {
		t.Errorf("AgentID = %q, want %q", sess.AgentID(), "test-agent-id-123")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run the agent session — the handler will echo the input.
	err = sess.Run(ctx, func(_ context.Context, input string) string {
		return "echo: " + input
	})
	// err should be context.DeadlineExceeded or context.Canceled.
	if err == nil {
		t.Error("Run should return an error when context is done or connection closes")
	}
}
