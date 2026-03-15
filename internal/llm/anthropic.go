package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// AnthropicProvider implements Provider using the Anthropic Messages API.
// It reads ANTHROPIC_API_KEY from the environment.
type AnthropicProvider struct {
	client anthropic.Client
}

func NewAnthropicProvider() *AnthropicProvider {
	return &AnthropicProvider{client: anthropic.NewClient()}
}

func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 1024
	}

	sdkMessages, err := convertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("converting messages: %w", err)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxTokens,
		Messages:  sdkMessages,
		Tools:     convertTools(req.Tools),
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.SystemPrompt}}
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic API: %w", err)
	}

	return convertResponse(msg), nil
}

func convertMessages(msgs []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks, err := convertContentToParams(m.Content)
		if err != nil {
			return nil, err
		}
		switch m.Role {
		case RoleUser:
			out = append(out, anthropic.NewUserMessage(blocks...))
		case RoleAssistant:
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		}
	}
	return out, nil
}

func convertContentToParams(contents []Content) ([]anthropic.ContentBlockParamUnion, error) {
	out := make([]anthropic.ContentBlockParamUnion, 0, len(contents))
	for _, c := range contents {
		switch c.Type {
		case ContentTypeText:
			out = append(out, anthropic.NewTextBlock(c.Text))
		case ContentTypeToolUse:
			out = append(out, anthropic.NewToolUseBlock(c.ID, json.RawMessage(c.Input), c.Name))
		case ContentTypeToolResult:
			out = append(out, anthropic.NewToolResultBlock(c.ToolUseID, c.Output, c.IsError))
		}
	}
	return out, nil
}

func convertTools(tools []ToolDef) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropic.ToolInputSchemaParam{}
		if len(t.InputSchema) > 0 {
			var m map[string]interface{}
			if err := json.Unmarshal(t.InputSchema, &m); err == nil {
				if props, ok := m["properties"]; ok {
					schema.Properties = props
				}
				if req, ok := m["required"].([]interface{}); ok {
					for _, r := range req {
						if s, ok := r.(string); ok {
							schema.Required = append(schema.Required, s)
						}
					}
				}
			}
		}
		tool := anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			tool.Description = param.NewOpt(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

func convertResponse(msg *anthropic.Message) *CompletionResponse {
	contents := make([]Content, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			contents = append(contents, Content{
				Type: ContentTypeText,
				Text: block.Text,
			})
		case "tool_use":
			contents = append(contents, Content{
				Type:  ContentTypeToolUse,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		case "thinking":
			contents = append(contents, Content{
				Type:     ContentTypeThinking,
				Thinking: block.Thinking,
			})
		}
	}
	return &CompletionResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: contents,
		},
		StopReason:   string(msg.StopReason),
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}
}
