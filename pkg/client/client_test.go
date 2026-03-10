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

// fakeServer is a minimal test server that handles the Slate JSON protocol.
type fakeServer struct {
	listener net.Listener
	// handlers map cmd → func(params json.RawMessage) (data string, err string)
	// data is the raw JSON to embed in "data"; err is non-empty for error responses.
	handlers map[string]func(params json.RawMessage) (string, string)
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &fakeServer{
		listener: ln,
		handlers: make(map[string]func(params json.RawMessage) (string, string)),
	}

	okHandler := func(_ json.RawMessage) (string, string) { return "", "" }

	// Default handlers.
	srv.handlers["health"] = okHandler
	srv.handlers["add_workspace"] = okHandler
	srv.handlers["del_workspace"] = okHandler
	srv.handlers["add_catalog"] = okHandler
	srv.handlers["del_catalog"] = okHandler
	srv.handlers["ls_catalogs"] = func(_ json.RawMessage) (string, string) {
		out, _ := json.Marshal(map[string]interface{}{
			"catalogs": []map[string]string{
				{"id": "cat1", "name": "test-cat"},
			},
		})
		return string(out), ""
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

		var req struct {
			Cmd    string          `json:"cmd"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			out, _ := json.Marshal(map[string]interface{}{"ok": false, "error": "invalid JSON"})
			conn.Write(append(out, '\n'))
			continue
		}
		cmd := strings.ToLower(req.Cmd)

		handler, ok := srv.handlers[cmd]
		var resp []byte
		if !ok {
			resp, _ = json.Marshal(map[string]interface{}{"ok": false, "error": fmt.Sprintf("unknown command: %s", cmd)})
		} else {
			data, errMsg := handler(req.Params)
			if errMsg != "" {
				resp, _ = json.Marshal(map[string]interface{}{"ok": false, "error": errMsg})
			} else if data == "" {
				resp, _ = json.Marshal(map[string]interface{}{"ok": true})
			} else {
				resp = []byte(fmt.Sprintf(`{"ok":true,"data":%s}`, data))
			}
		}
		conn.Write(append(resp, '\n'))
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

	if err := c.Health(); err != nil {
		t.Fatalf("Health: %v", err)
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
	// Override handler to return an error.
	srv.handlers["add_workspace"] = func(_ json.RawMessage) (string, string) {
		return "", "workspace already exists"
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

		// Read the register_agent command (JSON envelope).
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		_ = line // consume register_agent line

		// Send {"ok":true,"data":{"agent_id":"test-agent-id-123"}} response.
		data, _ := json.Marshal(map[string]string{"agent_id": "test-agent-id-123"})
		resp, _ := json.Marshal(map[string]interface{}{"ok": true, "data": json.RawMessage(data)})
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
