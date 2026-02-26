//go:build integration

// Package provider contains integration tests for Azure Codex.
//
// Run with: go test -tags=integration -v ./internal/provider/...
//
// Required environment variables:
//   - AZURE_OPENAI_API_KEY
//   - AZURE_OPENAI_ENDPOINT
//   - AZURE_OPENAI_CODEX=true
//   - AZURE_OPENAI_CODEX_MODEL (optional, defaults to gpt-5.2-codex)
//
// TODO: Create GitHub issue to track integration test improvements:
//   - Add VCR-style recorded responses for CI without credentials
//   - Add Docker-based E2E tests for full server testing
//   - Add performance benchmarks for token throughput
//   - Consider contract testing for API schema validation
package provider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func skipIfNoCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set, skipping integration test")
	}
	if os.Getenv("AZURE_OPENAI_ENDPOINT") == "" {
		t.Skip("AZURE_OPENAI_ENDPOINT not set, skipping integration test")
	}
	if os.Getenv("AZURE_OPENAI_CODEX") != "true" {
		t.Skip("AZURE_OPENAI_CODEX not set to true, skipping integration test")
	}
}

func getTestConfig() *Config {
	model := os.Getenv("AZURE_OPENAI_CODEX_MODEL")
	if model == "" {
		model = "gpt-5.2-codex"
	}
	apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")
	if apiVersion == "" {
		apiVersion = "2025-04-01-preview"
	}

	return &Config{
		Backend: BackendAzure,
		AzureOpenAI: ProviderAzureOpenAI{
			APIKey:     os.Getenv("AZURE_OPENAI_API_KEY"),
			Endpoint:   os.Getenv("AZURE_OPENAI_ENDPOINT"),
			APIVersion: apiVersion,
			Codex: &Codex{
				Enabled:              true,
				Model:                model,
				DefaultMaxTokens:     CodexDefaultMaxTokens,
				DefaultContext:       CodexDefaultContextTokens,
				HardMaxTokens:        CodexHardMaxTokens,
				HardMaxContextTokens: CodexHardMaxContextTokens,
			},
		},
		Tuning: SharedTuning{
			MaxTokens:   4096,
			Temperature: 0.2,
		},
	}
}

func TestIntegration_AzureCodex_SimpleChat(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	client, err := newAzureCodex(ctx, cfg)
	if err != nil {
		t.Fatalf("newAzureCodex() error = %v", err)
	}

	messages := []*schema.Message{
		schema.UserMessage("What is Terraform? Reply in one sentence."),
	}

	resp, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Content == "" {
		t.Error("expected non-empty content")
	}
	if resp.Role != schema.Assistant {
		t.Errorf("expected role %q, got %q", schema.Assistant, resp.Role)
	}

	t.Logf("Response: %s", resp.Content)
}

func TestIntegration_AzureCodex_TerraformGeneration(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := getTestConfig()
	client, err := newAzureCodex(ctx, cfg)
	if err != nil {
		t.Fatalf("newAzureCodex() error = %v", err)
	}

	messages := []*schema.Message{
		schema.UserMessage("Generate a simple Terraform configuration for an AWS S3 bucket with versioning enabled. Only output the HCL code, no explanation."),
	}

	resp, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Verify response contains Terraform-like content
	content := strings.ToLower(resp.Content)
	if !strings.Contains(content, "resource") && !strings.Contains(content, "aws_s3_bucket") {
		t.Errorf("expected Terraform resource in response, got: %s", resp.Content[:min(200, len(resp.Content))])
	}

	t.Logf("Generated Terraform:\n%s", resp.Content)
}

func TestIntegration_AzureCodex_ToolCalling(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	model, err := newAzureCodex(ctx, cfg)
	if err != nil {
		t.Fatalf("newAzureCodex() error = %v", err)
	}

	// Cast to concrete type for tool binding
	client, ok := model.(*azureCodexClient)
	if !ok {
		t.Fatal("expected *azureCodexClient")
	}

	// Bind a test tool
	tools := []*schema.ToolInfo{
		{
			Name: "terraform_plan",
			Desc: "Run terraform plan to preview infrastructure changes",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"directory": {
					Type:     "string",
					Desc:     "Directory containing Terraform configuration",
					Required: true,
				},
			}),
		},
	}

	if err := client.BindTools(tools); err != nil {
		t.Fatalf("BindTools() error = %v", err)
	}

	messages := []*schema.Message{
		schema.UserMessage("Run terraform plan in the /tmp/my-infra directory"),
	}

	resp, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// The model should either call the tool or explain it would call it
	hasToolCall := len(resp.ToolCalls) > 0
	mentionsPlan := strings.Contains(strings.ToLower(resp.Content), "plan") ||
		strings.Contains(strings.ToLower(resp.Content), "terraform")

	if !hasToolCall && !mentionsPlan {
		t.Errorf("expected tool call or mention of terraform plan, got: %s", resp.Content)
	}

	if hasToolCall {
		t.Logf("Tool call: %+v", resp.ToolCalls[0])
	} else {
		t.Logf("Response: %s", resp.Content)
	}
}

func TestIntegration_AzureCodex_Stream(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := getTestConfig()
	client, err := newAzureCodex(ctx, cfg)
	if err != nil {
		t.Fatalf("newAzureCodex() error = %v", err)
	}

	messages := []*schema.Message{
		schema.UserMessage("Say hello in exactly 3 words."),
	}

	reader, err := client.Stream(ctx, messages)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()

	// Note: Stream currently falls back to Generate (CODEX-1 limitation)
	// so we only get one message
	msg, err := reader.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Content == "" {
		t.Error("expected non-empty content")
	}

	t.Logf("Streamed response: %s", msg.Content)
}

func TestIntegration_AzureCodex_MultiTurn(t *testing.T) {
	skipIfNoCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := getTestConfig()
	client, err := newAzureCodex(ctx, cfg)
	if err != nil {
		t.Fatalf("newAzureCodex() error = %v", err)
	}

	// First turn
	messages := []*schema.Message{
		schema.UserMessage("I'm going to ask you about AWS. First, what does S3 stand for?"),
	}

	resp1, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate() turn 1 error = %v", err)
	}

	if !strings.Contains(strings.ToLower(resp1.Content), "simple storage service") &&
		!strings.Contains(strings.ToLower(resp1.Content), "s3") {
		t.Logf("Turn 1 response may not contain expected content: %s", resp1.Content)
	}

	// Second turn - add previous exchange to context
	messages = append(messages, resp1)
	messages = append(messages, schema.UserMessage("Now, what Terraform resource type would I use to create one?"))

	resp2, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate() turn 2 error = %v", err)
	}

	// Should reference aws_s3_bucket based on context
	if !strings.Contains(strings.ToLower(resp2.Content), "aws_s3_bucket") &&
		!strings.Contains(strings.ToLower(resp2.Content), "s3") {
		t.Errorf("expected reference to aws_s3_bucket, got: %s", resp2.Content)
	}

	t.Logf("Turn 1: %s", resp1.Content[:min(100, len(resp1.Content))])
	t.Logf("Turn 2: %s", resp2.Content[:min(100, len(resp2.Content))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
