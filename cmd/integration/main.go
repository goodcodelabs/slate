// Integration tests for the Slate server.
// Requires a running server at localhost:4242 (make run).
// Exits non-zero if any test fails.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slate/pkg/client"
	"strings"
	"sync"
	"time"
)

const serverAddr = "localhost:4242"

// ---- test harness ----

type suite struct {
	c    *client.Client
	pass int
	fail int
}

func (s *suite) section(name string) {
	fmt.Printf("\n--- %s ---\n", name)
}

func (s *suite) run(name string, fn func() error) bool {
	err := fn()
	if err != nil {
		fmt.Printf("  FAIL  %s\n        %v\n", name, err)
		s.fail++
		return false
	}
	fmt.Printf("  PASS  %s\n", name)
	s.pass++
	return true
}

func (s *suite) record(name string, err error) bool {
	return s.run(name, func() error { return err })
}

// mustError asserts that err is non-nil (expected failure path).
func mustError(err error) error {
	if err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	return nil
}

// mustContain asserts that s contains sub.
func mustContain(s, sub string) error {
	if !strings.Contains(s, sub) {
		return fmt.Errorf("expected %q to contain %q", s, sub)
	}
	return nil
}

// validJSON asserts that s parses as JSON.
func validJSON(s string) error {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return fmt.Errorf("invalid JSON: %w (got %q)", err, s)
	}
	return nil
}

// ---- main ----

func main() {
	c, err := client.Dial(serverAddr)
	if err != nil {
		log.Fatalf("cannot connect to %s: %v\n  Is the server running? (make run)", serverAddr, err)
	}
	defer func() { _ = c.Close() }()

	s := &suite{c: c}

	testHealth(s)
	testWorkspaces(s)
	testCatalogs(s)
	testAgents(s)
	testJobs(s)
	testManagement(s)
	testExternalAgent(s)

	fmt.Printf("\n%d passed, %d failed\n", s.pass, s.fail)
	if s.fail > 0 {
		os.Exit(1)
	}
}

// ---- test sections ----

func testHealth(s *suite) {
	s.section("Health")

	s.run("health returns ok", func() error {
		resp, err := s.c.Health()
		if err != nil {
			return err
		}
		return mustContain(resp, "ok")
	})
}

func testWorkspaces(s *suite) {
	s.section("Workspaces")

	s.run("add workspace", func() error {
		return s.c.AddWorkspace("integration-ws")
	})
	s.run("add duplicate workspace returns error", func() error {
		return mustError(s.c.AddWorkspace("integration-ws"))
	})
	s.run("delete workspace", func() error {
		return s.c.DelWorkspace("integration-ws")
	})
	s.run("delete missing workspace returns error", func() error {
		return mustError(s.c.DelWorkspace("integration-ws"))
	})
}

func testCatalogs(s *suite) {
	s.section("Catalogs")

	s.run("add catalog", func() error {
		return s.c.AddCatalog("integration-catalog")
	})
	s.run("list catalogs includes new catalog", func() error {
		catalogs, err := s.c.ListCatalogs()
		if err != nil {
			return err
		}
		for _, c := range catalogs {
			if c.Name == "integration-catalog" {
				return nil
			}
		}
		return fmt.Errorf("integration-catalog not found in %v", catalogs)
	})
	s.run("add duplicate catalog returns error", func() error {
		return mustError(s.c.AddCatalog("integration-catalog"))
	})
	s.run("delete catalog", func() error {
		return s.c.DelCatalog("integration-catalog")
	})
	s.run("delete missing catalog returns error", func() error {
		return mustError(s.c.DelCatalog("integration-catalog"))
	})
}

func testAgents(s *suite) {
	s.section("Agents")

	if err := s.c.AddCatalog("agent-test-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("agent-test-catalog") }()

	catalogID := findCatalogID(s, "agent-test-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	var agentID string
	if !s.run("add agent", func() error {
		info, err := s.c.AddAgent(catalogID, "test-agent")
		if err != nil {
			return err
		}
		if info.ID == "" {
			return fmt.Errorf("empty agent ID in response")
		}
		agentID = info.ID
		return nil
	}) {
		fmt.Println("  SKIP  (remaining agent tests: no agent ID)")
		return
	}

	s.run("set agent instructions", func() error {
		return s.c.SetAgentInstructions(agentID, "You are a test agent.")
	})
	s.run("set agent model", func() error {
		return s.c.SetAgentModel(agentID, "claude-haiku-4-5-20251001")
	})

	s.run("add tool to agent", func() error {
		return s.c.AddTool(agentID, "http_fetch")
	})
	s.run("list tools includes http_fetch", func() error {
		tools, err := s.c.ListTools(agentID)
		if err != nil {
			return err
		}
		for _, t := range tools {
			if t == "http_fetch" {
				return nil
			}
		}
		return fmt.Errorf("http_fetch not found in %v", tools)
	})
	s.run("add duplicate tool returns error", func() error {
		return mustError(s.c.AddTool(agentID, "http_fetch"))
	})
	s.run("remove tool", func() error {
		return s.c.RemoveTool(agentID, "http_fetch")
	})
	s.run("list tools is empty after removal", func() error {
		tools, err := s.c.ListTools(agentID)
		if err != nil {
			return err
		}
		if len(tools) != 0 {
			return fmt.Errorf("expected 0 tools, got %d: %v", len(tools), tools)
		}
		return nil
	})
	s.run("remove missing tool returns error", func() error {
		return mustError(s.c.RemoveTool(agentID, "http_fetch"))
	})
}

func testJobs(s *suite) {
	s.section("Jobs")

	s.run("list all jobs returns array", func() error {
		_, err := s.c.ListJobs("")
		return err
	})
}

func testManagement(s *suite) {
	s.section("Management")

	s.run("system_metrics returns valid JSON", func() error {
		resp, err := s.c.SystemMetrics()
		if err != nil {
			return err
		}
		return validJSON(resp)
	})
	s.run("system_stats returns valid JSON", func() error {
		resp, err := s.c.SystemStats()
		if err != nil {
			return err
		}
		return validJSON(resp)
	})
}

func testExternalAgent(s *suite) {
	s.section("External Agent")

	if err := s.c.AddCatalog("ext-agent-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("ext-agent-catalog") }()

	catalogID := findCatalogID(s, "ext-agent-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	registeredID := make(chan string, 1)
	sessionErr := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sess, err := client.DialAgentSession(serverAddr, catalogID, "echo-agent", "Echo the input back.")
		if err != nil {
			sessionErr <- err
			return
		}
		defer func() { _ = sess.Close() }()
		registeredID <- sess.AgentID()
		_ = sess.Run(ctx, func(_ context.Context, input string) string {
			return "echo: " + input
		})
	}()

	var agentID string
	select {
	case id := <-registeredID:
		agentID = id
		s.record("register external agent", nil)
	case err := <-sessionErr:
		s.record("register external agent", err)
		cancel()
		wg.Wait()
		return
	case <-time.After(5 * time.Second):
		s.record("register external agent", fmt.Errorf("timeout waiting for registration"))
		cancel()
		wg.Wait()
		return
	}

	s.run("run external agent returns echo", func() error {
		resp, err := s.c.RunAgent(agentID, "hello world")
		if err != nil {
			return err
		}
		return mustContain(resp, "echo")
	})
	s.run("run external agent preserves input", func() error {
		resp, err := s.c.RunAgent(agentID, "ping")
		if err != nil {
			return err
		}
		return mustContain(resp, "ping")
	})

	cancel()
	wg.Wait()
}

// ---- helpers ----

// findCatalogID returns the ID of the catalog with the given name, or "".
func findCatalogID(s *suite, name string) string {
	catalogs, err := s.c.ListCatalogs()
	if err != nil {
		return ""
	}
	for _, c := range catalogs {
		if c.Name == name {
			return c.ID
		}
	}
	return ""
}
