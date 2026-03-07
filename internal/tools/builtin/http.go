package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"slate/internal/llm"
)

const maxResponseBytes = 20 * 1024 // 20 KB — keeps each fetch well under token budget

// HTTPFetchTool fetches a URL and returns the response body.
type HTTPFetchTool struct {
	client *http.Client
}

func NewHTTPFetchTool() *HTTPFetchTool {
	return &HTTPFetchTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *HTTPFetchTool) Definition() llm.ToolDef {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"url":    {"type": "string", "description": "URL to fetch"},
			"method": {"type": "string", "enum": ["GET","POST","PUT","DELETE"], "description": "HTTP method (default GET)"},
			"body":   {"type": "string", "description": "Request body (for POST/PUT)"},
			"headers": {
				"type": "object",
				"description": "Additional HTTP headers as key-value pairs",
				"additionalProperties": {"type": "string"}
			}
		},
		"required": ["url"]
	}`)
	return llm.ToolDef{
		Name:        "http_fetch",
		Description: "Fetch content from a URL. Returns the response status code and body.",
		InputSchema: schema,
	}
}

func (t *HTTPFetchTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if params.Method == "" {
		params.Method = "GET"
	}

	var bodyReader io.Reader
	if params.Body != "" {
		bodyReader = strings.NewReader(params.Body)
	}

	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	truncated := len(bodyBytes) > maxResponseBytes
	if truncated {
		bodyBytes = bodyBytes[:maxResponseBytes]
	}

	result := map[string]interface{}{
		"status": resp.StatusCode,
		"body":   string(bodyBytes),
	}
	if truncated {
		result["truncated"] = true
		result["note"] = fmt.Sprintf("response body was truncated to %d bytes", maxResponseBytes)
	}

	out, _ := json.Marshal(result)
	return out, nil
}
