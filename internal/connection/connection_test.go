package connection_test

import (
	"bufio"
	"context"
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
		ClientIdleTimeout: 5000, // 5s for tests
	})
	return h, clientConn
}

// sendAndRead sends a command line and reads the next response line.
func sendAndRead(t *testing.T, conn net.Conn, cmd string) string {
	t.Helper()
	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return strings.TrimRight(line, "\r\n")
}

func TestHandleConnection_Health(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendAndRead(t, clientConn, "health")
	if resp != "ok" {
		t.Errorf("health response = %q, want %q", resp, "ok")
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

	// Drain all server writes so net.Pipe() writes don't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := clientConn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Send quit — handler should return.
	if _, err := clientConn.Write([]byte("quit\n")); err != nil {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendAndRead(t, clientConn, "not_a_real_command")
	if !strings.HasPrefix(resp, "error|") {
		t.Errorf("expected error| prefix, got %q", resp)
	}
}

func TestHandleConnection_AddWorkspace(t *testing.T) {
	store := newTestDB(t)
	handler, clientConn := newTestHandler(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	resp := sendAndRead(t, clientConn, "add_workspace test-ws")
	if resp != "ok" {
		t.Errorf("add_workspace response = %q, want %q", resp, "ok")
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

	// Drain all server writes so net.Pipe() writes don't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := clientConn.Read(buf)
			if err != nil {
				return
			}
		}
	}()
	defer clientConn.Close()

	// Cancel context — handler should exit.
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go handler.HandleConnection(ctx)
	defer clientConn.Close()

	reader := bufio.NewReader(clientConn)

	// Add a catalog.
	clientConn.Write([]byte("add_catalog my-cat\n"))
	line, _ := reader.ReadString('\n')
	if strings.TrimRight(line, "\r\n") != "ok" {
		t.Errorf("add_catalog = %q, want 'ok'", strings.TrimRight(line, "\r\n"))
	}

	// List catalogs.
	clientConn.Write([]byte("ls_catalogs\n"))
	line, _ = reader.ReadString('\n')
	resp := strings.TrimRight(line, "\r\n")
	if !strings.Contains(resp, "my-cat") {
		t.Errorf("ls_catalogs response %q does not contain 'my-cat'", resp)
	}
}
