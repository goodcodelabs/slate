package builtin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"slate/internal/tools/builtin"
)

func TestHTTPFetchTool_Definition(t *testing.T) {
	tool := builtin.NewHTTPFetchTool()
	def := tool.Definition()

	if def.Name != "http_fetch" {
		t.Errorf("Name = %q, want %q", def.Name, "http_fetch")
	}
	if def.Description == "" {
		t.Error("Description should not be empty")
	}
	if def.InputSchema == nil {
		t.Error("InputSchema should not be nil")
	}
}

func TestHTTPFetchTool_Execute_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	tool := builtin.NewHTTPFetchTool()
	input := json.RawMessage(`{"url":"` + srv.URL + `"}`)
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Status, http.StatusOK)
	}
	if result.Body != "hello world" {
		t.Errorf("body = %q, want %q", result.Body, "hello world")
	}
}

func TestHTTPFetchTool_Execute_POST_WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer srv.Close()

	tool := builtin.NewHTTPFetchTool()
	inputStr := `{"url":"` + srv.URL + `","method":"POST","body":"test body"}`
	out, err := tool.Execute(context.Background(), json.RawMessage(inputStr))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.Status != http.StatusCreated {
		t.Errorf("status = %d, want %d", result.Status, http.StatusCreated)
	}
}

func TestHTTPFetchTool_Execute_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	tool := builtin.NewHTTPFetchTool()
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute: %v (non-OK status should not be an error)", err)
	}

	var result struct {
		Status int `json:"status"`
	}
	json.Unmarshal(out, &result)
	if result.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", result.Status, http.StatusNotFound)
	}
}

func TestHTTPFetchTool_Execute_InvalidJSON(t *testing.T) {
	tool := builtin.NewHTTPFetchTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`not valid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON input, got nil")
	}
}

func TestHTTPFetchTool_Execute_Truncated(t *testing.T) {
	// Serve a body larger than the 20 KB cap.
	bigBody := make([]byte, 25*1024)
	for i := range bigBody {
		bigBody[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bigBody)
	}))
	defer srv.Close()

	tool := builtin.NewHTTPFetchTool()
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+srv.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result struct {
		Status    int    `json:"status"`
		Body      string `json:"body"`
		Truncated bool   `json:"truncated"`
		Note      string `json:"note"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("status = %d, want %d", result.Status, http.StatusOK)
	}
	if !result.Truncated {
		t.Error("expected truncated=true for oversized response")
	}
	if result.Note == "" {
		t.Error("expected note to be set when truncated")
	}
	if len(result.Body) != 20*1024 {
		t.Errorf("body len = %d, want %d", len(result.Body), 20*1024)
	}
}

func TestHTTPFetchTool_Execute_WithHeaders(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := builtin.NewHTTPFetchTool()
	inputStr := `{"url":"` + srv.URL + `","headers":{"X-Custom":"test-value"}}`
	_, err := tool.Execute(context.Background(), json.RawMessage(inputStr))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedHeader != "test-value" {
		t.Errorf("X-Custom header = %q, want %q", receivedHeader, "test-value")
	}
}
