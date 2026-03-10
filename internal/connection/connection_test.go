package connection_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"slate/internal/agent"
	"slate/internal/connection"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/scheduler"
)

func newTestDB(t *testing.T) *data.Data {
	t.Helper()
	db, err := data.New("test", t.TempDir())
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newTestHandler creates a Handler connected via net.Pipe() and returns the
// client-side connection for sending/reading in tests.
func newTestHandler(t *testing.T, store *data.Data) (handler *connection.Handler, clientConn net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()

	sched := scheduler.NewScheduler(2)
	sched.Start()
	t.Cleanup(sched.Stop)

	met := metrics.New()
	extAgents := agent.NewExternalAgentRegistry()

	h := connection.New(serverConn, sched, store, nil, met, extAgents, &connection.Options{
		ClientIdleTimeout: 5000,
	})
	return h, clientConn
}

// sendJSON sends a JSON command and reads the next JSON response line.
func sendJSON(t *testing.T, conn net.Conn, reader *bufio.Reader, cmd string, params interface{}) map[string]interface{} {
	t.Helper()
	req, _ := json.Marshal(map[string]interface{}{"cmd": cmd, "params": params})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimRight(line, "\r\n")), &result); err != nil {
		t.Fatalf("parse response: %v (got %q)", err, line)
	}
	return result
}

func isOK(resp map[string]interface{}) bool {
	ok, _ := resp["ok"].(bool)
	return ok
}

func TestHandleConnection_Health(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendJSON(t, clientConn, reader, "health", map[string]string{})
	if !isOK(resp) {
		t.Errorf("health response = %v, want ok:true", resp)
	}
}

func TestHandleConnection_Quit(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		handler.HandleConnection(ctx)
		close(done)
	}()

	// Drain server writes.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := clientConn.Read(buf); err != nil {
				return
			}
		}
	}()

	req, _ := json.Marshal(map[string]interface{}{"cmd": "quit", "params": map[string]string{}})
	if _, err := clientConn.Write(append(req, '\n')); err != nil {
		t.Fatalf("write quit: %v", err)
	}

	select {
	case <-done:
		// Handler exited as expected.
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after quit")
	}
	clientConn.Close()
}

func TestHandleConnection_InvalidCommand(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendJSON(t, clientConn, reader, "not_a_real_command", map[string]string{})
	if isOK(resp) {
		t.Errorf("expected ok:false for unknown command, got %v", resp)
	}
	if _, hasErr := resp["error"]; !hasErr {
		t.Errorf("expected error field, got %v", resp)
	}
}

func TestHandleConnection_AddWorkspace(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendJSON(t, clientConn, reader, "add_workspace", map[string]string{"name": "test-ws"})
	if !isOK(resp) {
		t.Errorf("add_workspace response = %v, want ok:true", resp)
	}
}

func TestHandleConnection_ContextCancellation(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		handler.HandleConnection(ctx)
		close(done)
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := clientConn.Read(buf); err != nil {
				return
			}
		}
	}()
	defer clientConn.Close()

	cancel()

	select {
	case <-done:
		// expected
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after context cancellation")
	}
}

func TestHandleConnection_AddAndListCatalogs(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	addResp := sendJSON(t, clientConn, reader, "add_catalog", map[string]string{"name": "my-cat"})
	if !isOK(addResp) {
		t.Errorf("add_catalog = %v, want ok:true", addResp)
	}

	listResp := sendJSON(t, clientConn, reader, "ls_catalogs", map[string]string{})
	if !isOK(listResp) {
		t.Errorf("ls_catalogs = %v, want ok:true", listResp)
	}
	out, _ := json.Marshal(listResp["data"])
	if !strings.Contains(string(out), "my-cat") {
		t.Errorf("ls_catalogs response %s does not contain 'my-cat'", out)
	}
}

func TestHandleConnection_InvalidJSON(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	// Send a non-JSON line.
	if _, err := clientConn.Write([]byte("not json at all\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	resp := strings.TrimRight(line, "\r\n")
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("response is not JSON: %q", resp)
	}
	ok, _ := result["ok"].(bool)
	if ok {
		t.Errorf("expected ok:false for invalid JSON, got ok:true")
	}
}

// Ensure responses match expected JSON structure.
func TestHandleConnection_ResponseEnvelope(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)
	reader := bufio.NewReader(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	// ls_workspaces should return {"ok":true,"data":{"workspaces":[]}}
	req, _ := json.Marshal(map[string]interface{}{"cmd": "ls_workspaces", "params": map[string]string{}})
	clientConn.Write(append(req, '\n'))

	line, _ := reader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")

	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("parse envelope: %v (got %q)", err, line)
	}
	if !env.OK {
		t.Errorf("expected ok:true, got false")
	}
	if string(env.Data) == "" {
		t.Error("expected data field to be present")
	}
	fmt.Printf("envelope data: %s\n", env.Data)
}
