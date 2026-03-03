package agent_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/agent"
)

func TestExternalAgentRegistry_Register_And_Get(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	id := ksuid.New()

	// Create an in-process connection pair.
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ac := reg.Register(id, serverConn)
	if ac == nil {
		t.Fatal("Register returned nil AgentConn")
	}

	got, ok := reg.Get(id)
	if !ok {
		t.Fatal("Get returned not-found for registered agent")
	}
	if got != ac {
		t.Error("Get returned different AgentConn than registered")
	}
}

func TestExternalAgentRegistry_Get_Missing(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	_, ok := reg.Get(ksuid.New())
	if ok {
		t.Fatal("expected not-found for unregistered agent ID")
	}
}

func TestExternalAgentRegistry_Unregister(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	id := ksuid.New()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	reg.Register(id, serverConn)
	reg.Unregister(id)

	_, ok := reg.Get(id)
	if ok {
		t.Fatal("expected not-found after unregister")
	}
}

func TestAgentConn_Run_SendsAndReceives(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	id := ksuid.New()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ac := reg.Register(id, serverConn)

	// Simulate an external agent on clientConn that echoes input.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := clientConn.Read(buf)
			if err != nil {
				return
			}
			var req struct {
				RunID string `json:"run_id"`
				Input string `json:"input"`
			}
			if err := json.Unmarshal(buf[:n-1], &req); err != nil {
				return
			}
			resp := map[string]string{
				"run_id":   req.RunID,
				"response": "echo: " + req.Input,
			}
			data, _ := json.Marshal(resp)
			_, _ = clientConn.Write(append(data, '\n'))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := ac.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "echo: hello" {
		t.Errorf("result = %q, want %q", result, "echo: hello")
	}
}

func TestAgentConn_Done_ClosedOnError(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	id := ksuid.New()

	serverConn, clientConn := net.Pipe()
	// Close clientConn immediately so serverConn reads fail.
	clientConn.Close()

	ac := reg.Register(id, serverConn)
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run should fail since the other end is closed.
	_, err := ac.Run(ctx, "hello")
	if err == nil {
		t.Fatal("expected error when reading from closed connection, got nil")
	}

	// Done channel should be closed.
	select {
	case <-ac.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Done channel not closed after connection error")
	}
}

func TestAgentConn_Run_AgentError(t *testing.T) {
	reg := agent.NewExternalAgentRegistry()
	id := ksuid.New()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ac := reg.Register(id, serverConn)

	// Simulate agent returning an error response.
	go func() {
		buf := make([]byte, 4096)
		n, err := clientConn.Read(buf)
		if err != nil {
			return
		}
		var req struct {
			RunID string `json:"run_id"`
		}
		json.Unmarshal(buf[:n-1], &req)

		resp := map[string]string{
			"run_id": req.RunID,
			"error":  "something went wrong",
		}
		data, _ := json.Marshal(resp)
		clientConn.Write(append(data, '\n'))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ac.Run(ctx, "test")
	if err == nil {
		t.Fatal("expected error from agent error response, got nil")
	}
}
