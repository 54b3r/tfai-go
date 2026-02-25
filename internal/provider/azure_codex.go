package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// azureCodexClient implements model.ToolCallingChatModel for GPT-5.2-Codex via raw HTTP.
// This uses the /openai/responses endpoint with Bearer auth instead of the standard
// Azure OpenAI chat completions endpoint.
//
// TODO: Add structured logging (request/response bodies at DEBUG level) when the
// logging overhaul is implemented. This is a raw HTTP client so debug visibility
// is important for troubleshooting.
type azureCodexClient struct {
	endpoint          string
	apiKey            string
	apiVersion        string
	modelName         string
	maxCompletionToks int
	httpClient        *http.Client
	boundTools        []*schema.ToolInfo
}

// codexRequest is the request body for the /openai/responses endpoint.
// Note: The Responses API uses "input" instead of "messages".
type codexRequest struct {
	Model               string         `json:"model"`
	Input               []codexMessage `json:"input"`
	MaxCompletionTokens int            `json:"max_output_tokens,omitempty"`
	Tools               []codexTool    `json:"tools,omitempty"`
	ToolChoice          any            `json:"tool_choice,omitempty"`
}

type codexMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []codexToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type codexTool struct {
	Type        string        `json:"type"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Parameters  any           `json:"parameters,omitempty"`
}

type codexToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function codexFunctionCall `json:"function"`
}

type codexFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// codexResponse is the response from the /openai/responses endpoint.
type codexResponse struct {
	ID     string              `json:"id"`
	Object string              `json:"object"`
	Status string              `json:"status"`
	Model  string              `json:"model"`
	Output []codexOutputItem   `json:"output"`
	Usage  codexUsage          `json:"usage"`
	Error  *codexError         `json:"error,omitempty"`
}

type codexOutputItem struct {
	ID      string               `json:"id"`
	Type    string               `json:"type"` // "reasoning" or "message"
	Status  string               `json:"status,omitempty"`
	Role    string               `json:"role,omitempty"`
	Content []codexContentBlock  `json:"content,omitempty"`
}

type codexContentBlock struct {
	Type      string          `json:"type"` // "output_text" or "tool_use"
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type codexError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// newAzureCodex constructs a ToolCallingChatModel backed by Azure AI Foundry GPT-5.2-Codex.
// This uses raw HTTP since SDKs don't yet support the /openai/responses endpoint.
// It reuses the AzureOpenAI config fields (APIKey, Endpoint, APIVersion) and adds
// CodexModel for the model name.
func newAzureCodex(_ context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	modelName := cfg.AzureOpenAI.CodexModel
	if modelName == "" {
		modelName = "gpt-5.2-codex"
	}
	apiVersion := cfg.AzureOpenAI.APIVersion
	if apiVersion == "" {
		apiVersion = "2025-04-01-preview"
	}

	return &azureCodexClient{
		endpoint:          cfg.AzureOpenAI.Endpoint,
		apiKey:            cfg.AzureOpenAI.APIKey,
		apiVersion:        apiVersion,
		modelName:         modelName,
		maxCompletionToks: cfg.Tuning.MaxTokens,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

// Generate implements model.ChatModel.
func (c *azureCodexClient) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// Convert eino messages to codex format
	codexMsgs := make([]codexMessage, 0, len(messages))
	for _, msg := range messages {
		cm := codexMessage{
			Role:    string(msg.Role),
			Content: extractContent(msg.Content),
		}
		// Handle tool call results
		if msg.Role == schema.Tool {
			cm.ToolCallID = msg.ToolCallID
		}
		// Handle assistant messages with tool calls
		if len(msg.ToolCalls) > 0 {
			cm.ToolCalls = make([]codexToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				cm.ToolCalls[i] = codexToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: codexFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
		codexMsgs = append(codexMsgs, cm)
	}

	req := codexRequest{
		Model:               c.modelName,
		Input:               codexMsgs,
		MaxCompletionTokens: c.maxCompletionToks,
	}

	// Apply bound tools
	if len(c.boundTools) > 0 {
		if converted := convertTools(c.boundTools); len(converted) > 0 {
			req.Tools = converted
		}
	}

	// Apply options (tools from options override bound tools)
	options := model.GetCommonOptions(&model.Options{}, opts...)
	if len(options.Tools) > 0 {
		if converted := convertTools(options.Tools); len(converted) > 0 {
			req.Tools = converted
		}
	}
	if options.ToolChoice != nil {
		req.ToolChoice = options.ToolChoice
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("codex API error: %s (type: %s, code: %s)", resp.Error.Message, resp.Error.Type, resp.Error.Code)
	}

	// Find the message output item (skip reasoning items)
	var messageItem *codexOutputItem
	for i := range resp.Output {
		if resp.Output[i].Type == "message" {
			messageItem = &resp.Output[i]
			break
		}
	}

	if messageItem == nil {
		return nil, fmt.Errorf("codex API returned no message output")
	}

	// Extract text content and tool calls from content blocks
	var textContent string
	var toolCalls []schema.ToolCall

	for _, block := range messageItem.Content {
		switch block.Type {
		case "output_text":
			textContent += block.Text
		case "tool_use", "function_call":
			toolCalls = append(toolCalls, schema.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      block.Name,
					Arguments: block.Arguments,
				},
			})
		}
	}

	result := &schema.Message{
		Role:      schema.RoleType(messageItem.Role),
		Content:   textContent,
		ToolCalls: toolCalls,
	}

	// Set response metadata
	result.ResponseMeta = &schema.ResponseMeta{
		FinishReason: resp.Status,
		Usage: &schema.TokenUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	return result, nil
}

// Stream implements model.ChatModel (streaming not yet supported, falls back to Generate).
func (c *azureCodexClient) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// For now, we simulate streaming by doing a blocking Generate call.
	// Full streaming can be added when the API supports it.
	msg, err := c.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	// Create a single-element stream
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		writer.Send(msg, nil)
	}()

	return reader, nil
}

// BindTools implements model.ToolCallingChatModel.
func (c *azureCodexClient) BindTools(tools []*schema.ToolInfo) error {
	c.boundTools = tools
	return nil
}

// WithTools implements model.ToolCallingChatModel - returns a new instance with tools bound.
func (c *azureCodexClient) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newClient := *c
	newClient.boundTools = tools
	return &newClient, nil
}

// convertTools converts eino ToolInfo to codex tool format.
// Filters out tools with empty names to avoid API errors.
func convertTools(tools []*schema.ToolInfo) []codexTool {
	result := make([]codexTool, 0, len(tools))
	for _, tool := range tools {
		// Skip tools with empty names
		if tool.Name == "" {
			continue
		}
		var params any
		if tool.ParamsOneOf != nil {
			// Convert to JSON schema for the API
			if jsonSchema, err := tool.ParamsOneOf.ToJSONSchema(); err == nil && jsonSchema != nil {
				params = jsonSchema
			}
		}
		result = append(result, codexTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Desc,
			Parameters:  params,
		})
	}
	return result
}

// doRequest sends the HTTP request to the codex endpoint.
func (c *azureCodexClient) doRequest(ctx context.Context, req codexRequest) (*codexResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("codex: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/responses?api-version=%s", c.endpoint, c.apiVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("codex: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("codex: request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("codex: failed to read response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex: HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp codexResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("codex: failed to parse response: %w", err)
	}

	return &resp, nil
}

// extractContent extracts string content from schema.Message content.
func extractContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		// Handle content blocks array
		var result string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						result += text
					}
				}
			}
		}
		return result
	default:
		return fmt.Sprintf("%v", v)
	}
}
