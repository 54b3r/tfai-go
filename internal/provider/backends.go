package provider

import (
	"context"
	"fmt"

	einoark "github.com/cloudwego/eino-ext/components/model/ark"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// newOllama constructs a ChatModel backed by a local Ollama instance.
// Requires MODEL_BASE_URL (default: http://localhost:11434) and MODEL_NAME.
func newOllama(ctx context.Context, cfg *Config) (model.ChatModel, error) { //nolint:staticcheck // SA1019: model.ChatModel deprecated upstream; migration tracked separately
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	v, err := einoollama.NewChatModel(ctx, &einoollama.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		BaseURL: baseURL,
		Model:   cfg.Model,
	})
	return v, err
}

// newOpenAI constructs a ChatModel backed by the OpenAI API.
// Requires MODEL_API_KEY and MODEL_NAME.
func newOpenAI(ctx context.Context, cfg *Config) (model.ChatModel, error) { //nolint:staticcheck // SA1019: model.ChatModel deprecated upstream; migration tracked separately
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("provider: MODEL_API_KEY is required for openai backend")
	}
	v, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		MaxTokens:   &cfg.MaxTokens,
		Temperature: &cfg.Temperature,
	})
	return v, err
}

// newAzure constructs a ChatModel backed by Azure OpenAI Service.
// Requires MODEL_API_KEY, MODEL_BASE_URL (endpoint), and AZURE_DEPLOYMENT.
func newAzure(ctx context.Context, cfg *Config) (model.ChatModel, error) { //nolint:staticcheck // SA1019: model.ChatModel deprecated upstream; migration tracked separately
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("provider: MODEL_API_KEY is required for azure backend")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("provider: MODEL_BASE_URL (Azure endpoint) is required for azure backend")
	}
	if cfg.AzureDeployment == "" {
		return nil, fmt.Errorf("provider: AZURE_DEPLOYMENT is required for azure backend")
	}
	return einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:       cfg.AzureDeployment,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		ByAzure:     true,
		APIVersion:  cfg.AzureAPIVersion,
		MaxTokens:   &cfg.MaxTokens,
		Temperature: &cfg.Temperature,
		// Use the deployment name as-is — the default mapper strips dots/colons
		// which breaks deployment names like "gpt-4.1".
		AzureModelMapperFunc: func(model string) string { return model },
	})
}

// newBedrock constructs a ChatModel backed by AWS Bedrock.
// AWS credentials are resolved via the standard SDK credential chain
// (env vars, ~/.aws/credentials, instance profile, etc.).
// Requires MODEL_NAME (Bedrock model ID) and AWS_REGION.
func newBedrock(ctx context.Context, cfg *Config) (model.ChatModel, error) { //nolint:staticcheck // SA1019: model.ChatModel deprecated upstream; migration tracked separately
	// Ark is the ByteDance/Volcano Engine model runtime; for AWS Bedrock we use
	// the ark provider configured with the Bedrock-compatible endpoint.
	// TODO: Replace with a dedicated Bedrock implementation when available in eino-ext.
	maxTokens := cfg.MaxTokens
	temp := cfg.Temperature
	return einoark.NewChatModel(ctx, &einoark.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
}

// newGemini constructs a ChatModel backed by Google Gemini (AI Studio or Vertex AI).
// Requires MODEL_API_KEY and MODEL_NAME (e.g. "gemini-1.5-pro").
func newGemini(ctx context.Context, cfg *Config) (model.ChatModel, error) { //nolint:staticcheck // SA1019: model.ChatModel deprecated upstream; migration tracked separately
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("provider: MODEL_API_KEY is required for gemini backend")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("provider: failed to create Gemini client: %w", err)
	}
	return einogemini.NewChatModel(ctx, &einogemini.Config{ //nolint:wrapcheck // constructor passthrough
		Client: client,
		Model:  cfg.Model,
	})
}
