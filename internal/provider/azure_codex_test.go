package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAzureCodexClient_Generate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serverResponse string
		serverStatus   int
		messages       []*schema.Message
		wantErr        string
		wantContent    string
	}{
		{
			name: "successful response",
			serverResponse: `{
				"id": "resp-123",
				"output": [
					{
						"type": "message",
						"role": "assistant",
						"content": [{"type": "output_text", "text": "Hello, world!"}]
					}
				],
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`,
			serverStatus: http.StatusOK,
			messages: []*schema.Message{
				schema.UserMessage("Hi"),
			},
			wantContent: "Hello, world!",
		},
		{
			name: "api error response",
			serverResponse: `{
				"error": {
					"type": "invalid_request_error",
					"message": "Invalid API key",
					"code": "401"
				}
			}`,
			serverStatus: http.StatusOK,
			messages: []*schema.Message{
				schema.UserMessage("Hi"),
			},
			wantErr: "Invalid API key",
		},
		{
			name:           "http error",
			serverResponse: `{"error": {"message": "Unauthorized"}}`,
			serverStatus:   http.StatusUnauthorized,
			messages: []*schema.Message{
				schema.UserMessage("Hi"),
			},
			wantErr: "HTTP 401",
		},
		{
			name: "empty output",
			serverResponse: `{
				"id": "resp-123",
				"output": [],
				"usage": {"input_tokens": 10, "output_tokens": 0}
			}`,
			serverStatus: http.StatusOK,
			messages: []*schema.Message{
				schema.UserMessage("Hi"),
			},
			wantErr: "no message output",
		},
		{
			name: "tool call response",
			serverResponse: `{
				"id": "resp-123",
				"output": [
					{
						"type": "message",
						"role": "assistant",
						"content": [
							{
								"type": "tool_use",
								"id": "call-123",
								"name": "terraform_plan",
								"input": "{\"dir\": \"/tmp\"}"
							}
						]
					}
				],
				"usage": {"input_tokens": 10, "output_tokens": 20}
			}`,
			serverStatus: http.StatusOK,
			messages: []*schema.Message{
				schema.UserMessage("Run terraform plan"),
			},
			wantContent: "", // Tool calls don't have text content
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if !strings.Contains(r.URL.Path, "/openai/responses") {
					t.Errorf("expected /openai/responses in path, got %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("expected Bearer auth, got %s", r.Header.Get("Authorization"))
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
				}

				// Verify request body
				var reqBody codexRequest
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}
				if reqBody.Model != "gpt-5.2-codex" {
					t.Errorf("expected model gpt-5.2-codex, got %s", reqBody.Model)
				}
				if len(reqBody.Input) != len(tc.messages) {
					t.Errorf("expected %d messages, got %d", len(tc.messages), len(reqBody.Input))
				}

				w.WriteHeader(tc.serverStatus)
				_, _ = w.Write([]byte(tc.serverResponse))
			}))
			defer server.Close()

			// Create client pointing to test server
			client := &azureCodexClient{
				endpoint:          server.URL,
				apiKey:            "test-key",
				apiVersion:        "2025-04-01-preview",
				modelName:         "gpt-5.2-codex",
				maxCompletionToks: 4096,
				httpClient:        server.Client(),
			}

			// Call Generate
			resp, err := client.Generate(context.Background(), tc.messages)

			// Check error
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check response
			if resp == nil {
				t.Fatal("expected non-nil response")
			}
			if tc.wantContent != "" && resp.Content != tc.wantContent {
				t.Errorf("content = %q, want %q", resp.Content, tc.wantContent)
			}
		})
	}
}

func TestAzureCodexClient_ConvertTools(t *testing.T) {
	t.Parallel()

	tools := []*schema.ToolInfo{
		{
			Name: "terraform_plan",
			Desc: "Run terraform plan",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"dir": {
					Type: "string",
					Desc: "Directory to run plan in",
				},
			}),
		},
		{
			Name: "terraform_state",
			Desc: "Get terraform state",
		},
	}

	codexTools := convertTools(tools)

	if len(codexTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(codexTools))
	}

	// Check first tool
	if codexTools[0].Name != "terraform_plan" {
		t.Errorf("tool[0].Name = %q, want %q", codexTools[0].Name, "terraform_plan")
	}
	if codexTools[0].Type != "function" {
		t.Errorf("tool[0].Type = %q, want %q", codexTools[0].Type, "function")
	}
	if codexTools[0].Description != "Run terraform plan" {
		t.Errorf("tool[0].Description = %q, want %q", codexTools[0].Description, "Run terraform plan")
	}

	// Check second tool
	if codexTools[1].Name != "terraform_state" {
		t.Errorf("tool[1].Name = %q, want %q", codexTools[1].Name, "terraform_state")
	}
}

func TestAzureCodexClient_BindTools(t *testing.T) {
	t.Parallel()

	client := &azureCodexClient{
		modelName: "gpt-5.2-codex",
	}

	tools := []*schema.ToolInfo{
		{Name: "test_tool", Desc: "A test tool"},
	}

	err := client.BindTools(tools)
	if err != nil {
		t.Fatalf("BindTools() error = %v", err)
	}

	if len(client.boundTools) != 1 {
		t.Errorf("expected 1 bound tool, got %d", len(client.boundTools))
	}
}

func TestAzureCodexClient_Stream(t *testing.T) {
	t.Parallel()

	// Stream falls back to Generate, so test the stream reader behavior
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "resp-123",
			"output": [
				{
					"type": "message",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "Streamed response"}]
				}
			],
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer server.Close()

	client := &azureCodexClient{
		endpoint:          server.URL,
		apiKey:            "test-key",
		apiVersion:        "2025-04-01-preview",
		modelName:         "gpt-5.2-codex",
		maxCompletionToks: 4096,
		httpClient:        server.Client(),
	}

	reader, err := client.Stream(context.Background(), []*schema.Message{
		schema.UserMessage("Hi"),
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()

	// Read from stream (should get exactly one message since we simulate streaming)
	msg, err := reader.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if msg.Content != "Streamed response" {
		t.Errorf("content = %q, want %q", msg.Content, "Streamed response")
	}
}

func TestNewAzureCodex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       *Config
		wantModel string
		wantVer   string
	}{
		{
			name: "default values",
			cfg: &Config{
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:   "key",
					Endpoint: "https://test.openai.azure.com",
					Codex:    &Codex{Enabled: true},
				},
			},
			wantModel: "gpt-5.2-codex",
			wantVer:   "2025-04-01-preview",
		},
		{
			name: "custom model",
			cfg: &Config{
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:   "key",
					Endpoint: "https://test.openai.azure.com",
					Codex: &Codex{
						Enabled: true,
						Model:   "gpt-6-codex",
					},
				},
			},
			wantModel: "gpt-6-codex",
			wantVer:   "2025-04-01-preview",
		},
		{
			name: "custom api version",
			cfg: &Config{
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:     "key",
					Endpoint:   "https://test.openai.azure.com",
					APIVersion: "2026-01-01-preview",
					Codex:      &Codex{Enabled: true, Model: "gpt-5.2-codex"},
				},
			},
			wantModel: "gpt-5.2-codex",
			wantVer:   "2026-01-01-preview",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model, err := newAzureCodex(context.Background(), tc.cfg)
			if err != nil {
				t.Fatalf("newAzureCodex() error = %v", err)
			}

			client, ok := model.(*azureCodexClient)
			if !ok {
				t.Fatal("expected *azureCodexClient")
			}

			if client.modelName != tc.wantModel {
				t.Errorf("modelName = %q, want %q", client.modelName, tc.wantModel)
			}
			if client.apiVersion != tc.wantVer {
				t.Errorf("apiVersion = %q, want %q", client.apiVersion, tc.wantVer)
			}
		})
	}
}
