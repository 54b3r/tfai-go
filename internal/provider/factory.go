package provider

import (
	"context"
	"fmt"
	"log/slog"
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

// ModelCfg is used to store a list of Models; Chat, Generate and Embedding modesl that store an Initialized Calling Model
type ModelCfg struct {
	ChatModel      model.ToolCallingChatModel
	GenerateModel  model.ToolCallingChatModel
	EmbeddingModel model.ToolCallingChatModel
}

func NewFromEnv(ctx context.Context) (*ModelCfg, error) {
	var genCfg *Config
	var genModel model.ToolCallingChatModel
	// var models []model.ToolCallingChatModel
	// mc := &ModelConfigs{}
	mc2 := &ModelCfg{}

	cfg := ConfigFromEnv()
	model, err := New(ctx, cfg)
	if err != nil {
		return mc2, fmt.Errorf("generate: failed to initialize chat model provider: %w", err)
	}

	// If cfg.Generate is present and the generate backend does not match the config backend (different model providers)
	// we will override the generate values
	if cfg.Generate != nil && cfg.Generate.Backend != cfg.Backend {
		genCfg = cfg.WithGenerateOverrides()
		genModel, err = New(ctx, genCfg)
		if err != nil {
			return mc2, fmt.Errorf("generate: failed to initialize generation model provider: %w", err)
		}
		mc2.GenerateModel = genModel
	} else {
		genModel = model
	}

	mc2.ChatModel = model
	mc2.GenerateModel = genModel
	mc2.EmbeddingModel = genModel // For now defaulting to gen model, we can inpl the embeddings once

	// Put models in an array to be extracted and used
	// models = append(models, mc.Type[0].ChatModel, mc.Type[0].GenerateModel, mc.Type[0].EmbeddingModel)
	// return models, err
	return mc2, err
}

// ConfigFromEnv builds a provider Config from environment variables without
// constructing a model. This is useful when callers need the config for
// ancillary purposes (e.g. building a HealthCheckConfig) in addition to
// creating a ChatModel.
func ConfigFromEnv() *Config {
	var defaultGenBackend Backend
	backend := Backend(getEnvOrDefault("MODEL_PROVIDER", string(BackendOllama)))
	genBackend := Backend(getEnvOrDefault("GENERATE_MODEL_PROVIDER", string(BackendOllama)))

	// if genBackend is empty or = configured model provider
	if genBackend == "" || genBackend == backend {
		defaultGenBackend = backend
	} else {
		defaultGenBackend = genBackend
	}

	cfg := &Config{
		Backend: backend,
		Generate: &GenerateOverrides{
			Backend:    defaultGenBackend,                                                 // Backend Confiuration
			Deployment: os.Getenv("AZURE_OPENAI_DEPLOYMENT"),                              // Azure OpenAI Extracted Value
			Version:    getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2025-04-01-preview"), // Azure OpenAI Extracted Value
			Model:      os.Getenv("GENERATE_MODEL"),                                       // OpenAI/Ollama/Gemini Extracted Value
			ModelID:    os.Getenv("GENERATE_MODEL_ID"),                                    // Bedrock Extracted Value
		},
		AzureOpenAI: ProviderAzureOpenAI{
			APIKey:            os.Getenv("AZURE_OPENAI_API_KEY"),
			Endpoint:          os.Getenv("AZURE_OPENAI_ENDPOINT"),
			Deployment:        os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
			APIVersion:        getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2025-04-01-preview"),
			ReasoningOverride: getEnvBoolPtr("AZURE_OPENAI_REASONING"),
		},
		Bedrock: ProviderBedrock{
			AWSRegion: getEnvOrDefault("AWS_REGION", "us-east-1"),
			ModelID:   os.Getenv("BEDROCK_MODEL_ID"),
		},
		Gemini: ProviderGemini{
			APIKey: os.Getenv("GOOGLE_API_KEY"),
			Model:  getEnvOrDefault("GEMINI_MODEL", "gemini-1.5-pro"),
		},
		OpenAI: ProviderOpenAI{
			APIKey: os.Getenv("OPENAI_API_KEY"),
			Model:  getEnvOrDefault("OPENAI_MODEL", "gpt-4o"),
		},
		Ollama: ProviderOllama{
			Host:  getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434"),
			Model: getEnvOrDefault("OLLAMA_MODEL", "llama3"),
		},
		Tuning: SharedTuning{
			MaxTokens:   getEnvInt("MODEL_MAX_TOKENS", 4096),
			Temperature: getEnvFloat32("MODEL_TEMPERATURE", 0.2),
		},
	}
	// // Generate Overrides if GENERATE_PROVIDER is set, otherwise use configured backend values for chat model
	// if generateBackend != "" {
	// 	cfg.Generate.Backend = Backend(generateBackend)
	// 	cfg.Generate.Deployment = os.Getenv("GENERATE_AZURE_DEPLOYMENT") // Azure OAI
	// 	cfg.Generate.Version = os.Getenv("GENERATE_AZURE_VERSION")       // Azure OAI
	// 	cfg.Generate.Model = os.Getenv("GENERATE_MODEL")                 // OpenAI/Ollama/Gemini
	// 	cfg.Generate.ModelID = os.Getenv("GENERATE_MODEL_ID")            // Bedrock
	// 	slog.Info("config: generate backend being overwriten with the following ",
	// 		slog.String("GENERATE_BACKEND", string(cfg.Generate.Backend)),
	// 		slog.String("GENERATE_AZURE_DEPLOYMENT", cfg.Generate.Deployment),
	// 		slog.String("GENERATE_AZURE_VERSION", cfg.Generate.Version),
	// 		slog.String("GENERATE_MODEL", cfg.Generate.Model),
	// 		slog.String("GENERATE_MODEL_ID", cfg.Generate.ModelID),
	// 	)
	// }
	return cfg
}

func (c *Config) WithGenerateOverrides() *Config {
	// Tells us if the operator is explicityly wanting to override the generate model provider
	// ie, we do NOT want to use the same chat model for code generation

	genBackend := os.Getenv("GENERATE_MODEL_PROVIDER")      // Override the default configured code generation model
	genDeployment := os.Getenv("GENERATE_AZURE_DEPLOYMENT") // Use different model deployed in Azure OpenAI/Foundry
	genVersion := os.Getenv("GENERATE_AZURE_VERSION")       // Use a different API Version for an Azure OpenAI Deployment
	genModelName := os.Getenv("GENERATE_MODEL")             // Use a different model for OpenAI/Ollama/Gemini providers
	genModelID := os.Getenv("GENERATE_MODEL_ID")            // Use a different modelBedrock

	// If no override values are extracted, noOverrideSet will be true.
	// This in combo with the empty backend extract will just return the original config object.
	noOverrideSet := genDeployment == "" && genVersion == "" && genModelName == "" && genModelID == ""

	// Delete this will never be nil - we always set sain defaults
	if genBackend == "" && noOverrideSet {
		slog.Info("WithGenerate: No Overrides values have been set, if you are intending to override the generate models please set and retry")
		return c // no overrides configured, return original
	}

	// Shallow copy the config to only override the Generate configs
	modified := *c

	// Check if Backends match, Override backend if specified
	// this should always be true - need to revalidate the code to make sure we cant just put it top level
	if c.Generate.Backend != "" {
		slog.Info("Generate model override not set") // Might be too verbose?
		if genBackend == "" {
			if c.Backend == c.Generate.Backend {
				slog.Info("Provider backends match, using " + string(c.Backend) + " provider.\nIf overriding other generate values, ensure you are setting the appropriate environment/yaml variables for configuration")
			}
		}
		modified.Backend = c.Generate.Backend
	}

	// Apply model-specific overrides based on target backend
	switch modified.Backend {
	case BackendAzure:
		if genDeployment != "" {
			modified.AzureOpenAI.Deployment = genDeployment
		}
		if genVersion != "" {
			modified.AzureOpenAI.APIVersion = genVersion
		}
	case BackendOpenAI:
		if genModelName != "" {
			modified.OpenAI.Model = genModelName
		}
	case BackendOllama:
		if genModelName != "" {
			modified.Ollama.Model = genModelName
		}
	case BackendGemini:
		if genModelName != "" {
			modified.Gemini.Model = genModelName
		}
	case BackendBedrock:
		if genModelID != "" {
			modified.Bedrock.ModelID = genModelID
		}
	}

	return &modified
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

// getEnvBoolPtr returns a *bool parsed from the named environment variable.
// Returns nil when the variable is unset or empty (signals: use auto-detection).
// Returns a pointer to true for "true", pointer to false for "false".
// Any other non-empty value is treated as false.
func getEnvBoolPtr(key string) *bool {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b := v == "true"
	return &b
}
