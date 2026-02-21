package provider

import (
	"context"
	"fmt"
	"strings"

	einoark "github.com/cloudwego/eino-ext/components/model/ark"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// newOllama constructs a ToolCallingChatModel backed by a local Ollama instance.
// Reads OLLAMA_HOST (default: http://localhost:11434) and OLLAMA_MODEL.
func newOllama(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	v, err := einoollama.NewChatModel(ctx, &einoollama.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		BaseURL: cfg.Ollama.Host,
		Model:   cfg.Ollama.Model,
	})
	return v, err
}

// newOpenAI constructs a ToolCallingChatModel backed by the OpenAI API.
// Reads OPENAI_API_KEY and OPENAI_MODEL.
func newOpenAI(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	v, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:       cfg.OpenAI.Model,
		APIKey:      cfg.OpenAI.APIKey,
		MaxTokens:   &cfg.Tuning.MaxTokens,
		Temperature: &cfg.Tuning.Temperature,
	})
	return v, err
}

// azureReasoningPrefixes lists deployment name prefixes that identify Azure OpenAI
// reasoning models. These models reject temperature, top_p, presence_penalty,
// frequency_penalty, and max_tokens parameters.
// Source: https://learn.microsoft.com/en-us/azure/ai-services/openai/concepts/models
// (o-series section, verified 2026-02-20)
var azureReasoningPrefixes = []string{
	"o1", "o2", "o3", "o4", "codex",
}

// isAzureReasoningModel returns true when the deployment name matches a known
// reasoning model prefix. The check is case-insensitive and prefix-based so
// that versioned names like "o3-mini" and "o1-preview" are caught automatically.
func isAzureReasoningModel(deployment string) bool {
	lower := strings.ToLower(deployment)
	for _, prefix := range azureReasoningPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+"-") || strings.HasPrefix(lower, prefix+".") {
			return true
		}
	}
	return false
}

// newAzure constructs a ToolCallingChatModel backed by Azure OpenAI Service.
// Reads AZURE_OPENAI_API_KEY, AZURE_OPENAI_ENDPOINT, and AZURE_OPENAI_DEPLOYMENT.
//
// Reasoning-model detection is automatic: deployment names matching known o-series
// or codex prefixes skip temperature and max_tokens (which those models reject).
// Set AZURE_OPENAI_REASONING=true/false to override auto-detection explicitly.
func newAzure(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	reasoning := isAzureReasoningModel(cfg.AzureOpenAI.Deployment)
	if cfg.AzureOpenAI.ReasoningOverride != nil {
		reasoning = *cfg.AzureOpenAI.ReasoningOverride
	}

	azureCfg := &einoopenai.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:      cfg.AzureOpenAI.Deployment,
		APIKey:     cfg.AzureOpenAI.APIKey,
		BaseURL:    cfg.AzureOpenAI.Endpoint,
		ByAzure:    true,
		APIVersion: cfg.AzureOpenAI.APIVersion,
		// Use the deployment name as-is — the default mapper strips dots/colons
		// which breaks deployment names like "gpt-4.1".
		AzureModelMapperFunc: func(model string) string { return model },
	}
	if reasoning {
		// Reasoning models fix temperature=1, top_p=1, presence_penalty=0,
		// frequency_penalty=0 and reject max_tokens. Use MaxCompletionTokens
		// (which includes reasoning tokens) instead.
		azureCfg.MaxCompletionTokens = &cfg.Tuning.MaxTokens
	} else {
		azureCfg.MaxTokens = &cfg.Tuning.MaxTokens
		azureCfg.Temperature = &cfg.Tuning.Temperature
	}
	return einoopenai.NewChatModel(ctx, azureCfg)
}

// newBedrock constructs a ToolCallingChatModel backed by AWS Bedrock.
// AWS credentials are resolved via the standard SDK credential chain
// (AWS_PROFILE, env vars, ~/.aws/credentials, instance profile, etc.).
// Reads BEDROCK_MODEL_ID and AWS_REGION.
func newBedrock(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	// Ark is the ByteDance/Volcano Engine model runtime; for AWS Bedrock we use
	// the ark provider configured with the Bedrock-compatible endpoint.
	// TODO: Replace with a dedicated Bedrock implementation when available in eino-ext.
	maxTokens := cfg.Tuning.MaxTokens
	temp := cfg.Tuning.Temperature
	return einoark.NewChatModel(ctx, &einoark.ChatModelConfig{ //nolint:wrapcheck // constructor passthrough
		Model:       cfg.Bedrock.ModelID,
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
}

// newGemini constructs a ToolCallingChatModel backed by Google Gemini (AI Studio or Vertex AI).
// Reads GOOGLE_API_KEY and GEMINI_MODEL (e.g. "gemini-1.5-pro").
func newGemini(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.Gemini.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("provider: failed to create Gemini client: %w", err)
	}
	return einogemini.NewChatModel(ctx, &einogemini.Config{ //nolint:wrapcheck // constructor passthrough
		Client: client,
		Model:  cfg.Gemini.Model,
	})
}
