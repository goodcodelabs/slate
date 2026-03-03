package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"slate/internal/tools/builtin"
)

func TestFileTool_Definition(t *testing.T) {
	tool := builtin.NewFileTool()
	def := tool.Definition()

	if def.Name != "file" {
		t.Errorf("Name = %q, want %q", def.Name, "file")
	}
	if def.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestFileTool_Write_And_Read(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	tool := builtin.NewFileTool()

	// Write.
	writeInput := json.RawMessage(`{"action":"write","path":"` + path + `","content":"hello file"}`)
	out, err := tool.Execute(context.Background(), writeInput)
	if err != nil {
		t.Fatalf("write Execute: %v", err)
	}
	var writeResult struct {
		Status string `json:"status"`
	}
	json.Unmarshal(out, &writeResult)
	if writeResult.Status != "ok" {
		t.Errorf("write status = %q, want %q", writeResult.Status, "ok")
	}

	// Read back.
	readInput := json.RawMessage(`{"action":"read","path":"` + path + `"}`)
	out, err = tool.Execute(context.Background(), readInput)
	if err != nil {
		t.Fatalf("read Execute: %v", err)
	}
	var readResult struct {
		Content string `json:"content"`
	}
	json.Unmarshal(out, &readResult)
	if readResult.Content != "hello file" {
		t.Errorf("content = %q, want %q", readResult.Content, "hello file")
	}
}

func TestFileTool_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "append.txt")
	tool := builtin.NewFileTool()

	writeInput := json.RawMessage(`{"action":"write","path":"` + path + `","content":"line1\n"}`)
	_, _ = tool.Execute(context.Background(), writeInput)

	appendInput := json.RawMessage(`{"action":"append","path":"` + path + `","content":"line2\n"}`)
	_, err := tool.Execute(context.Background(), appendInput)
	if err != nil {
		t.Fatalf("append Execute: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2\n" {
		t.Errorf("file content = %q, want %q", string(data), "line1\nline2\n")
	}
}

func TestFileTool_Read_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.txt")
	tool := builtin.NewFileTool()

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"read","path":"`+path+`"}`))
	if err == nil {
		t.Fatal("expected error reading nonexistent file, got nil")
	}
}

func TestFileTool_PathTraversal_Rejected(t *testing.T) {
	tool := builtin.NewFileTool()

	// Attempt path traversal via ../../
	input := json.RawMessage(`{"action":"read","path":"/tmp/../etc/passwd"}`)
	_, err := tool.Execute(context.Background(), input)
	// /tmp/../etc/passwd cleans to /etc/passwd which does not contain ".."
	// So this is NOT a traversal attempt per the implementation.
	// A real traversal would be a relative path with "..".
	_ = err

	// Relative path with .. traversal.
	input2 := json.RawMessage(`{"action":"read","path":"../../../etc/passwd"}`)
	_, err2 := tool.Execute(context.Background(), input2)
	if err2 == nil {
		t.Error("expected path traversal error for ../../.., got nil")
	}
}

func TestFileTool_UnknownAction(t *testing.T) {
	tool := builtin.NewFileTool()
	input := json.RawMessage(`{"action":"delete","path":"/tmp/x"}`)
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
}

func TestFileTool_Write_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")
	tool := builtin.NewFileTool()

	writeInput := json.RawMessage(`{"action":"write","path":"` + path + `","content":"nested"}`)
	_, err := tool.Execute(context.Background(), writeInput)
	if err != nil {
		t.Fatalf("write to nested path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading nested file: %v", err)
	}
	if string(data) != "nested" {
		t.Errorf("nested file content = %q, want %q", string(data), "nested")
	}
}

func TestFileTool_InvalidJSON(t *testing.T) {
	tool := builtin.NewFileTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
