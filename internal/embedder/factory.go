package embedder

import (
	"fmt"
	"os"
	"strconv"

	"github.com/54b3r/tfai-go/internal/rag"
)

// Default embedding models per backend.
const (
	defaultOllamaModel  = "nomic-embed-text"
	defaultOpenAIModel  = "text-embedding-3-small"
	defaultBedrockModel = "amazon.titan-embed-text-v2"
	defaultGeminiModel  = "text-embedding-004"

	// defaultOllamaDimensions is the output dimension of nomic-embed-text.
	// Other Ollama models may differ — override with EMBEDDING_DIMENSIONS.
	defaultOllamaDimensions = 768
	// defaultOpenAIDimensions is the output dimension of text-embedding-3-small.
	defaultOpenAIDimensions = 1536
)

// DefaultDimensions returns the correct default embedding vector size for the
// given backend name. Callers that need to pre-configure a vector store (e.g.
// Qdrant collection creation) should use this rather than hardcoding a value.
// EMBEDDING_DIMENSIONS always takes precedence when set.
func DefaultDimensions(backend string) int {
	if v := getEnvInt("EMBEDDING_DIMENSIONS", 0); v > 0 {
		return v
	}
	switch backend {
	case "ollama":
		return defaultOllamaDimensions
	default:
		return defaultOpenAIDimensions
	}
}

// NewFromEnv constructs a rag.Embedder using cascading defaults that inherit
// from the chat provider configuration when embedding-specific overrides are
// not set.
//
// Resolution order:
//
//  1. EMBEDDING_PROVIDER — if unset, inherits MODEL_PROVIDER (default: ollama)
//  2. Per-backend credentials are inherited from the chat provider's env vars
//  3. EMBEDDING_MODEL — overrides the default model for the resolved backend
//  4. EMBEDDING_API_KEY — overrides the inherited API key
//  5. EMBEDDING_ENDPOINT — overrides the inherited endpoint
//  6. EMBEDDING_DIMENSIONS — overrides the default dimensions (ollama: 768, openai/azure: 1536)
func NewFromEnv() (rag.Embedder, error) {
	// 1. Resolve provider — fall back to MODEL_PROVIDER, then "ollama".
	backend := getEnv("EMBEDDING_PROVIDER")
	if backend == "" {
		backend = getEnvOrDefault("MODEL_PROVIDER", "ollama")
	}

	switch backend {
	case "ollama":
		host := getEnv("EMBEDDING_ENDPOINT")
		if host == "" {
			host = getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434")
		}
		model := getEnvOrDefault("EMBEDDING_MODEL", defaultOllamaModel)
		return NewOllamaEmbedder(&OllamaConfig{
			Host:  host,
			Model: model,
		}), nil

	case "openai":
		dims := getEnvInt("EMBEDDING_DIMENSIONS", defaultOpenAIDimensions)
		apiKey := getEnv("EMBEDDING_API_KEY")
		if apiKey == "" {
			apiKey = getEnv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("embedder: openai requires OPENAI_API_KEY or EMBEDDING_API_KEY")
		}
		baseURL := getEnv("EMBEDDING_ENDPOINT")
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		model := getEnvOrDefault("EMBEDDING_MODEL", defaultOpenAIModel)
		return NewOpenAIEmbedder(&OpenAIConfig{
			BaseURL:    baseURL,
			APIKey:     apiKey,
			Model:      model,
			Dimensions: dims,
		}), nil

	case "azure":
		dims := getEnvInt("EMBEDDING_DIMENSIONS", defaultOpenAIDimensions)
		apiKey := getEnv("EMBEDDING_API_KEY")
		if apiKey == "" {
			apiKey = getEnv("AZURE_OPENAI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("embedder: azure requires AZURE_OPENAI_API_KEY or EMBEDDING_API_KEY")
		}
		endpoint := getEnv("EMBEDDING_ENDPOINT")
		if endpoint == "" {
			endpoint = getEnv("AZURE_OPENAI_ENDPOINT")
		}
		if endpoint == "" {
			return nil, fmt.Errorf("embedder: azure requires AZURE_OPENAI_ENDPOINT or EMBEDDING_ENDPOINT")
		}
		apiVersion := getEnvOrDefault("AZURE_OPENAI_API_VERSION", "2025-04-01-preview")
		model := getEnvOrDefault("EMBEDDING_MODEL", defaultOpenAIModel)
		return NewOpenAIEmbedder(&OpenAIConfig{
			BaseURL:    endpoint + "/openai",
			APIKey:     apiKey,
			Model:      model,
			Dimensions: dims,
			Azure:      true,
			APIVersion: apiVersion,
		}), nil

	case "bedrock":
		// Future: implement BedrockEmbedder. For now, return an error.
		return nil, fmt.Errorf("embedder: bedrock embedding support is not yet implemented (model: %s)", defaultBedrockModel)

	case "gemini":
		// Future: implement GeminiEmbedder. For now, return an error.
		return nil, fmt.Errorf("embedder: gemini embedding support is not yet implemented (model: %s)", defaultGeminiModel)

	default:
		return nil, fmt.Errorf("embedder: unknown backend %q — valid values: ollama, openai, azure, bedrock, gemini", backend)
	}
}

// getEnv returns the value of the named environment variable, or empty string.
func getEnv(key string) string {
	return os.Getenv(key)
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
