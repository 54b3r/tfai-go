package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/cloudwego/eino/components/model"
)

// NewFromEnv constructs a ChatModel by reading provider configuration from
// environment variables. The MODEL_PROVIDER variable selects the backend;
// remaining variables are provider-specific.
//
// Environment variables:
//
//	MODEL_PROVIDER      = ollama | openai | azure | bedrock | gemini  (default: ollama)
//	MODEL_NAME          = model/deployment name
//	MODEL_BASE_URL      = base URL override (Ollama, Azure)
//	MODEL_API_KEY       = API key (OpenAI, Azure, Gemini)
//	AZURE_DEPLOYMENT    = Azure OpenAI deployment name
//	AWS_REGION          = AWS region for Bedrock
//	MODEL_MAX_TOKENS    = max tokens (default: 4096)
//	MODEL_TEMPERATURE   = temperature 0.0–1.0 (default: 0.2)
func NewFromEnv(ctx context.Context) (model.ChatModel, error) {
	cfg := &Config{
		Backend:         Backend(getEnvOrDefault("MODEL_PROVIDER", string(BackendOllama))),
		Model:           getEnvOrDefault("MODEL_NAME", "llama3"),
		BaseURL:         os.Getenv("MODEL_BASE_URL"),
		APIKey:          os.Getenv("MODEL_API_KEY"),
		AzureDeployment: os.Getenv("AZURE_DEPLOYMENT"),
		AzureAPIVersion: getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2024-02-01"),
		AWSRegion:       getEnvOrDefault("AWS_REGION", "us-east-1"),
		MaxTokens:       getEnvInt("MODEL_MAX_TOKENS", 4096),
		Temperature:     getEnvFloat32("MODEL_TEMPERATURE", 0.2),
	}

	return New(ctx, cfg)
}

// New constructs a ChatModel from an explicit Config, delegating to the
// appropriate backend factory function.
func New(ctx context.Context, cfg *Config) (model.ChatModel, error) {
	switch cfg.Backend {
	case BackendOllama:
		return newOllama(ctx, cfg)
	case BackendOpenAI:
		return newOpenAI(ctx, cfg)
	case BackendAzure:
		return newAzure(ctx, cfg)
	case BackendBedrock:
		return newBedrock(ctx, cfg)
	case BackendGemini:
		return newGemini(ctx, cfg)
	default:
		return nil, fmt.Errorf("provider: unknown backend %q — valid values: ollama, openai, azure, bedrock, gemini", cfg.Backend)
	}
}

// getEnvOrDefault returns the value of the named environment variable, or
// fallback if the variable is unset or empty.
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns the integer value of the named environment variable, or
// fallback if the variable is unset, empty, or not parseable.
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

// getEnvFloat32 returns the float32 value of the named environment variable,
// or fallback if the variable is unset, empty, or not parseable.
func getEnvFloat32(key string, fallback float32) float32 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil {
			return float32(f)
		}
	}
	return fallback
}
