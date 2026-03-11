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

// validJSON asserts that raw parses as JSON.
func validJSON(raw json.RawMessage) error {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w (got %q)", err, string(raw))
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
	testDelAgent(s)
	testWorkspaceConfiguration(s)
	testAgentThreads(s)
	testPipelines(s)
	testJobs(s)
	testManagement(s)
	testExternalAgent(s)
	testJobLifecycle(s)
	testAgentThreadChat(s)

	fmt.Printf("\n%d passed, %d failed\n", s.pass, s.fail)
	if s.fail > 0 {
		os.Exit(1)
	}
}

// ---- test sections ----

func testHealth(s *suite) {
	s.section("Health")

	s.run("health returns ok", func() error {
		return s.c.Health()
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

func testAgentThreads(s *suite) {
	s.section("Agent Threads")

	if err := s.c.AddCatalog("at-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("at-catalog") }()

	catalogID := findCatalogID(s, "at-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	var agentID string
	if !s.run("add agent for thread tests", func() error {
		info, err := s.c.AddAgent(catalogID, "at-agent")
		if err != nil {
			return err
		}
		agentID = info.ID
		return nil
	}) {
		fmt.Println("  SKIP  (remaining agent thread tests: no agent ID)")
		return
	}

	var threadID string

	s.run("create agent thread with name", func() error {
		info, err := s.c.NewAgentThread(agentID, "my-thread")
		if err != nil {
			return err
		}
		if info.ID == "" {
			return fmt.Errorf("empty thread ID")
		}
		if info.Name != "my-thread" {
			return fmt.Errorf("name = %q, want %q", info.Name, "my-thread")
		}
		threadID = info.ID
		return nil
	})

	s.run("create agent thread without name auto-generates one", func() error {
		info, err := s.c.NewAgentThread(agentID, "")
		if err != nil {
			return err
		}
		if info.Name == "" {
			return fmt.Errorf("expected auto-generated name, got empty string")
		}
		return nil
	})

	s.run("list agent threads returns created threads", func() error {
		threads, err := s.c.ListAgentThreads(agentID)
		if err != nil {
			return err
		}
		if len(threads) < 2 {
			return fmt.Errorf("expected at least 2 threads, got %d", len(threads))
		}
		return nil
	})

	if threadID != "" {
		s.run("agent thread history is initially empty", func() error {
			raw, err := s.c.AgentThreadHistory(threadID)
			if err != nil {
				return err
			}
			return mustContain(string(raw), "messages")
		})
	}

	s.run("create agent thread with nonexistent agent returns error", func() error {
		return mustError(func() error {
			_, err := s.c.NewAgentThread("2cDKvMGSMqCjFpuSkNdRaR7EiSa", "bad")
			return err
		}())
	})

	s.run("list agent threads for nonexistent agent returns error", func() error {
		return mustError(func() error {
			_, err := s.c.ListAgentThreads("2cDKvMGSMqCjFpuSkNdRaR7EiSa")
			return err
		}())
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
		jobID, err := s.c.RunAgent(agentID, "hello world")
		if err != nil {
			return err
		}
		result, err := s.c.WaitJob(jobID)
		if err != nil {
			return err
		}
		return mustContain(result.Result, "echo")
	})
	s.run("run external agent preserves input", func() error {
		jobID, err := s.c.RunAgent(agentID, "ping")
		if err != nil {
			return err
		}
		result, err := s.c.WaitJob(jobID)
		if err != nil {
			return err
		}
		return mustContain(result.Result, "ping")
	})

	cancel()
	wg.Wait()
}

func testDelAgent(s *suite) {
	s.section("Delete Agent")

	if err := s.c.AddCatalog("del-agent-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("del-agent-catalog") }()

	catalogID := findCatalogID(s, "del-agent-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	var agentID string
	if !s.run("add agent to delete", func() error {
		info, err := s.c.AddAgent(catalogID, "doomed-agent")
		if err != nil {
			return err
		}
		agentID = info.ID
		return nil
	}) {
		return
	}

	s.run("set instructions on agent before deletion", func() error {
		return s.c.SetAgentInstructions(agentID, "you will be deleted")
	})
	s.run("delete agent succeeds", func() error {
		return s.c.DelAgent(agentID)
	})
	s.run("delete agent again returns error", func() error {
		return mustError(s.c.DelAgent(agentID))
	})
	s.run("new agent thread for deleted agent returns error", func() error {
		return mustError(func() error {
			_, err := s.c.NewAgentThread(agentID, "orphan")
			return err
		}())
	})
	s.run("delete agent with invalid id returns error", func() error {
		return mustError(s.c.DelAgent("not-a-valid-ksuid"))
	})
}

func testWorkspaceConfiguration(s *suite) {
	s.section("Workspace Configuration")

	if err := s.c.AddWorkspace("config-ws"); err != nil {
		fmt.Printf("  SKIP  (setup: add workspace failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelWorkspace("config-ws") }()

	if err := s.c.AddCatalog("config-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("config-catalog") }()

	wsID := findWorkspaceID(s, "config-ws")
	if wsID == "" {
		fmt.Println("  SKIP  (setup: workspace ID not found)")
		return
	}
	catalogID := findCatalogID(s, "config-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	agentInfo, err := s.c.AddAgent(catalogID, "router-agent")
	if err != nil {
		fmt.Printf("  SKIP  (setup: add agent failed: %v)\n", err)
		return
	}

	s.run("list workspaces includes config-ws", func() error {
		workspaces, err := s.c.ListWorkspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			if w.Name == "config-ws" {
				return nil
			}
		}
		return fmt.Errorf("config-ws not found in %v", workspaces)
	})
	s.run("set workspace catalog", func() error {
		return s.c.SetWorkspaceCatalog(wsID, catalogID)
	})
	s.run("set workspace router", func() error {
		return s.c.SetWorkspaceRouter(wsID, agentInfo.ID)
	})
	s.run("list workspaces shows catalog and router after configuration", func() error {
		workspaces, err := s.c.ListWorkspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			if w.Name == "config-ws" {
				if w.CatalogID == "" {
					return fmt.Errorf("expected catalog_id to be set, got empty")
				}
				if w.RouterAgentID == "" {
					return fmt.Errorf("expected router_agent_id to be set, got empty")
				}
				return nil
			}
		}
		return fmt.Errorf("config-ws not found")
	})
	s.run("set workspace catalog with nonexistent workspace returns error", func() error {
		return mustError(s.c.SetWorkspaceCatalog("2cDKvMGSMqCjFpuSkNdRaR7EiSa", catalogID))
	})
	s.run("set workspace router with nonexistent agent returns error", func() error {
		return mustError(s.c.SetWorkspaceRouter(wsID, "2cDKvMGSMqCjFpuSkNdRaR7EiSa"))
	})
}

func testPipelines(s *suite) {
	s.section("Pipelines")

	if err := s.c.AddWorkspace("pipeline-ws"); err != nil {
		fmt.Printf("  SKIP  (setup: add workspace failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelWorkspace("pipeline-ws") }()

	if err := s.c.AddCatalog("pipeline-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("pipeline-catalog") }()

	wsID := findWorkspaceID(s, "pipeline-ws")
	if wsID == "" {
		fmt.Println("  SKIP  (setup: workspace ID not found)")
		return
	}
	catalogID := findCatalogID(s, "pipeline-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	agent1, err := s.c.AddAgent(catalogID, "step-agent-1")
	if err != nil {
		fmt.Printf("  SKIP  (setup: add agent failed: %v)\n", err)
		return
	}
	agent2, err := s.c.AddAgent(catalogID, "step-agent-2")
	if err != nil {
		fmt.Printf("  SKIP  (setup: add agent 2 failed: %v)\n", err)
		return
	}

	var pipelineID string
	if !s.run("create pipeline", func() error {
		info, err := s.c.CreatePipeline(wsID, "my-pipeline")
		if err != nil {
			return err
		}
		if info.ID == "" {
			return fmt.Errorf("empty pipeline ID in response")
		}
		pipelineID = info.ID
		return nil
	}) {
		return
	}

	s.run("add sequential step to pipeline", func() error {
		return s.c.AddPipelineStep(pipelineID, agent1.ID, "sequential")
	})
	s.run("add parallel step to pipeline", func() error {
		return s.c.AddPipelineStep(pipelineID, agent2.ID, "parallel")
	})
	s.run("add pipeline step with invalid mode returns error", func() error {
		return mustError(s.c.AddPipelineStep(pipelineID, agent1.ID, "diagonal"))
	})
	s.run("create pipeline with nonexistent workspace returns error", func() error {
		return mustError(func() error {
			_, err := s.c.CreatePipeline("2cDKvMGSMqCjFpuSkNdRaR7EiSa", "bad")
			return err
		}())
	})
	s.run("run pipeline returns job id", func() error {
		jobID, err := s.c.RunPipeline(pipelineID, "test input")
		if err != nil {
			return err
		}
		if jobID == "" {
			return fmt.Errorf("expected non-empty job ID")
		}
		return nil
	})
}

func testJobLifecycle(s *suite) {
	s.section("Job Lifecycle")

	if err := s.c.AddCatalog("job-lifecycle-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("job-lifecycle-catalog") }()

	catalogID := findCatalogID(s, "job-lifecycle-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	registeredID := make(chan string, 1)
	sessionErr := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sess, err := client.DialAgentSession(serverAddr, catalogID, "lifecycle-echo-agent", "Echo the input.")
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
		s.record("register echo agent for lifecycle tests", nil)
	case err := <-sessionErr:
		s.record("register echo agent for lifecycle tests", err)
		cancel()
		wg.Wait()
		return
	case <-time.After(5 * time.Second):
		s.record("register echo agent for lifecycle tests", fmt.Errorf("timeout"))
		cancel()
		wg.Wait()
		return
	}

	var jobID string
	if !s.run("run agent returns job id", func() error {
		id, err := s.c.RunAgent(agentID, "lifecycle test")
		if err != nil {
			return err
		}
		jobID = id
		return nil
	}) {
		cancel()
		wg.Wait()
		return
	}

	s.run("wait job returns completed result", func() error {
		result, err := s.c.WaitJob(jobID)
		if err != nil {
			return err
		}
		if result.Status != "completed" {
			return fmt.Errorf("status = %q, want %q", result.Status, "completed")
		}
		return mustContain(result.Result, "echo")
	})
	s.run("job status shows completed", func() error {
		status, err := s.c.JobStatus(jobID)
		if err != nil {
			return err
		}
		if status.Status != "completed" {
			return fmt.Errorf("status = %q, want %q", status.Status, "completed")
		}
		if status.CreatedAt == "" {
			return fmt.Errorf("expected non-empty created_at")
		}
		return nil
	})
	s.run("job result returns completed result", func() error {
		result, err := s.c.JobResult(jobID)
		if err != nil {
			return err
		}
		if result.Status != "completed" {
			return fmt.Errorf("status = %q, want %q", result.Status, "completed")
		}
		return mustContain(result.Result, "echo")
	})
	s.run("cancel completed job is idempotent", func() error {
		return s.c.CancelJob(jobID)
	})
	s.run("cancel nonexistent job returns error", func() error {
		return mustError(s.c.CancelJob("2cDKvMGSMqCjFpuSkNdRaR7EiSa"))
	})
	s.run("job status for nonexistent job returns error", func() error {
		return mustError(func() error {
			_, err := s.c.JobStatus("2cDKvMGSMqCjFpuSkNdRaR7EiSa")
			return err
		}())
	})
	s.run("ls jobs includes completed job", func() error {
		jobs, err := s.c.ListJobs("")
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.ID == jobID {
				return nil
			}
		}
		return fmt.Errorf("job %s not found in ls_jobs response", jobID)
	})

	cancel()
	wg.Wait()
}

func testAgentThreadChat(s *suite) {
	s.section("Agent Thread Chat")

	if err := s.c.AddCatalog("thread-chat-catalog"); err != nil {
		fmt.Printf("  SKIP  (setup: add catalog failed: %v)\n", err)
		return
	}
	defer func() { _ = s.c.DelCatalog("thread-chat-catalog") }()

	catalogID := findCatalogID(s, "thread-chat-catalog")
	if catalogID == "" {
		fmt.Println("  SKIP  (setup: catalog ID not found)")
		return
	}

	registeredID := make(chan string, 1)
	sessionErr := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sess, err := client.DialAgentSession(serverAddr, catalogID, "thread-echo-agent", "Echo the input.")
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
		s.record("register echo agent for thread chat", nil)
	case err := <-sessionErr:
		s.record("register echo agent for thread chat", err)
		cancel()
		wg.Wait()
		return
	case <-time.After(5 * time.Second):
		s.record("register echo agent for thread chat", fmt.Errorf("timeout"))
		cancel()
		wg.Wait()
		return
	}

	var threadID string
	if !s.run("create agent thread for echo agent", func() error {
		info, err := s.c.NewAgentThread(agentID, "chat-thread")
		if err != nil {
			return err
		}
		threadID = info.ID
		return nil
	}) {
		cancel()
		wg.Wait()
		return
	}

	s.run("agent chat returns job id", func() error {
		jobID, err := s.c.AgentChat(threadID, "hello")
		if err != nil {
			return err
		}
		result, err := s.c.WaitJob(jobID)
		if err != nil {
			return err
		}
		if result.Status != "completed" {
			return fmt.Errorf("status = %q, want %q", result.Status, "completed")
		}
		return mustContain(result.Result, "echo")
	})
	s.run("agent thread history has 2 messages after first turn", func() error {
		raw, err := s.c.AgentThreadHistory(threadID)
		if err != nil {
			return err
		}
		var out struct {
			Messages []interface{} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("parse history: %w", err)
		}
		if len(out.Messages) != 2 {
			return fmt.Errorf("expected 2 messages, got %d", len(out.Messages))
		}
		return nil
	})
	s.run("second agent chat accumulates history", func() error {
		jobID, err := s.c.AgentChat(threadID, "world")
		if err != nil {
			return err
		}
		result, err := s.c.WaitJob(jobID)
		if err != nil {
			return err
		}
		return mustContain(result.Result, "echo")
	})
	s.run("agent thread history has 4 messages after second turn", func() error {
		raw, err := s.c.AgentThreadHistory(threadID)
		if err != nil {
			return err
		}
		var out struct {
			Messages []interface{} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("parse history: %w", err)
		}
		if len(out.Messages) != 4 {
			return fmt.Errorf("expected 4 messages, got %d", len(out.Messages))
		}
		return nil
	})
	s.run("agent chat on nonexistent thread returns error", func() error {
		return mustError(func() error {
			_, err := s.c.AgentChat("2cDKvMGSMqCjFpuSkNdRaR7EiSa", "hello")
			return err
		}())
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

// findWorkspaceID returns the ID of the workspace with the given name, or "".
func findWorkspaceID(s *suite, name string) string {
	workspaces, err := s.c.ListWorkspaces()
	if err != nil {
		return ""
	}
	for _, w := range workspaces {
		if w.Name == name {
			return w.ID
		}
	}
	return ""
}
