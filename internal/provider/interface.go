// Package provider defines the ModelProvider interface and factory for
// selecting and constructing LLM backend implementations at runtime.
// Supported backends: Ollama, OpenAI, Azure OpenAI, AWS Bedrock, Google Gemini.
package provider

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

// Backend enumerates the supported LLM inference providers.
type Backend string

const (
	// BackendOllama selects a locally running Ollama instance.
	BackendOllama Backend = "ollama"
	// BackendOpenAI selects the OpenAI API.
	BackendOpenAI Backend = "openai"
	// BackendAzure selects Azure OpenAI Service.
	BackendAzure Backend = "azure"
	// BackendBedrock selects AWS Bedrock.
	BackendBedrock Backend = "bedrock"
	// BackendGemini selects Google Gemini via Vertex AI or AI Studio.
	BackendGemini Backend = "gemini"
)

// Config holds all provider-level configuration resolved from environment
// variables or explicit caller-supplied values.
type Config struct {
	// Backend identifies which inference provider to use.
	Backend Backend

	// Model is the model name or deployment ID to use (e.g. "gpt-4o", "llama3").
	Model string

	// BaseURL overrides the default API endpoint (required for Ollama and Azure).
	BaseURL string

	// APIKey is the authentication credential for the selected provider.
	// For Bedrock this field is unused; AWS credentials are resolved via the SDK chain.
	APIKey string

	// AzureDeployment is the Azure OpenAI deployment name (Azure only).
	AzureDeployment string

	// AzureAPIVersion is the Azure OpenAI REST API version (Azure only).
	// Populated from AZURE_OPENAI_API_VERSION (e.g. "2024-02-01").
	AzureAPIVersion string

	// AWSRegion is the AWS region for Bedrock (Bedrock only).
	AWSRegion string

	// MaxTokens caps the number of tokens the model may generate per response.
	MaxTokens int

	// Temperature controls response randomness (0.0–1.0).
	Temperature float32
}

// Factory is the interface for constructing a ChatModel from a Config.
// Implementations must be safe to call from multiple goroutines.
type Factory interface {
	// New constructs and returns a ready-to-use ChatModel for the given config.
	New(ctx context.Context, cfg *Config) (model.ChatModel, error)
}
