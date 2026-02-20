package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/cloudwego/eino/components/model"
)

// NewFromEnv constructs a ChatModel by reading provider configuration from
// environment variables. MODEL_PROVIDER selects the backend; each provider
// uses its own native credential env vars.
//
// Environment variables:
//
//	MODEL_PROVIDER              = ollama | openai | azure | bedrock | gemini (default: ollama)
//
//	Ollama:  OLLAMA_HOST (default: http://localhost:11434), OLLAMA_MODEL (default: llama3)
//	OpenAI:  OPENAI_API_KEY, OPENAI_MODEL (default: gpt-4o)
//	Azure:   AZURE_OPENAI_API_KEY, AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_DEPLOYMENT,
//	         AZURE_OPENAI_API_VERSION (default: 2024-02-01)
//	Bedrock: AWS credential chain (AWS_PROFILE / AWS_ACCESS_KEY_ID+AWS_SECRET_ACCESS_KEY /
//	         instance profile), AWS_REGION (default: us-east-1), BEDROCK_MODEL_ID
//	Gemini:  GOOGLE_API_KEY, GEMINI_MODEL (default: gemini-1.5-pro)
//
//	Shared:  MODEL_MAX_TOKENS (default: 4096), MODEL_TEMPERATURE (default: 0.2)
func NewFromEnv(ctx context.Context) (model.ToolCallingChatModel, error) {
	cfg := &Config{
		Backend: Backend(getEnvOrDefault("MODEL_PROVIDER", string(BackendOllama))),
		Ollama: ProviderOllama{
			Host:  getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434"),
			Model: getEnvOrDefault("OLLAMA_MODEL", "llama3"),
		},
		OpenAI: ProviderOpenAI{
			APIKey: os.Getenv("OPENAI_API_KEY"),
			Model:  getEnvOrDefault("OPENAI_MODEL", "gpt-4o"),
		},
		AzureOpenAI: ProviderAzureOpenAI{
			APIKey:     os.Getenv("AZURE_OPENAI_API_KEY"),
			Endpoint:   os.Getenv("AZURE_OPENAI_ENDPOINT"),
			Deployment: os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
			APIVersion: getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2024-02-01"),
		},
		Bedrock: ProviderBedrock{
			AWSRegion: getEnvOrDefault("AWS_REGION", "us-east-1"),
			ModelID:   os.Getenv("BEDROCK_MODEL_ID"),
		},
		Gemini: ProviderGemini{
			APIKey: os.Getenv("GOOGLE_API_KEY"),
			Model:  getEnvOrDefault("GEMINI_MODEL", "gemini-1.5-pro"),
		},
		Tuning: SharedTuning{
			MaxTokens:   getEnvInt("MODEL_MAX_TOKENS", 4096),
			Temperature: getEnvFloat32("MODEL_TEMPERATURE", 0.2),
		},
	}

	return New(ctx, cfg)
}

// New constructs a ChatModel from an explicit Config, delegating to the
// appropriate backend factory function. It validates the config first so
// callers get a clear error at startup rather than on the first request.
func New(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
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
