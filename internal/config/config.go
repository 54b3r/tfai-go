// Package config provides YAML-based configuration for tfai.
// Configuration is loaded with a layered precedence: defaults → YAML file → env vars.
// Environment variables always win, so existing workflows are unaffected.
//
// File search order:
//  1. --config CLI flag (explicit path)
//  2. TFAI_CONFIG environment variable
//  3. ~/.tfai/config.yaml
//  4. ./tfai.yaml
//
// If no file is found the system runs entirely from env vars (backwards compatible).
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config is the top-level YAML configuration structure.
// Field names use yaml tags that mirror the env var naming (lowercase, underscored).
type Config struct {
	// Model configures the LLM chat model provider.
	Model ModelConfig `yaml:"model"`

	// Embedding configures the embedding provider for RAG.
	Embedding EmbeddingConfig `yaml:"embedding"`

	// Qdrant configures the Qdrant vector store connection.
	Qdrant QdrantConfig `yaml:"qdrant"`

	// Server configures the HTTP server.
	Server ServerConfig `yaml:"server"`

	// Logging configures structured logging.
	Logging LoggingConfig `yaml:"logging"`

	// History configures conversation history persistence.
	History HistoryConfig `yaml:"history"`

	// Tracing configures Langfuse tracing integration.
	Tracing TracingConfig `yaml:"tracing"`
}

// ModelConfig holds LLM chat model settings.
type ModelConfig struct {
	// Provider selects the backend: ollama, openai, azure, bedrock, gemini.
	Provider string `yaml:"provider"`

	// MaxTokens is the maximum number of tokens in the response.
	MaxTokens int `yaml:"max_tokens"`

	// Temperature controls response randomness (0.0–1.0).
	Temperature float32 `yaml:"temperature"`

	// Ollama holds Ollama-specific settings.
	Ollama OllamaConfig `yaml:"ollama"`

	// OpenAI holds OpenAI-specific settings.
	OpenAI OpenAIConfig `yaml:"openai"`

	// Azure holds Azure OpenAI-specific settings.
	Azure AzureConfig `yaml:"azure"`

	// Bedrock holds AWS Bedrock-specific settings.
	Bedrock BedrockConfig `yaml:"bedrock"`

	// Gemini holds Google Gemini-specific settings.
	Gemini GeminiConfig `yaml:"gemini"`
}

// OllamaConfig holds Ollama provider settings.
type OllamaConfig struct {
	// Host is the Ollama API endpoint.
	Host string `yaml:"host"`
	// Model is the Ollama model name.
	Model string `yaml:"model"`
}

// OpenAIConfig holds OpenAI provider settings.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key. Prefer env var OPENAI_API_KEY.
	APIKey string `yaml:"api_key"`
	// Model is the OpenAI model name.
	Model string `yaml:"model"`
}

// AzureConfig holds Azure OpenAI provider settings.
type AzureConfig struct {
	// APIKey is the Azure OpenAI API key. Prefer env var AZURE_OPENAI_API_KEY.
	APIKey string `yaml:"api_key"`
	// Endpoint is the Azure OpenAI resource endpoint.
	Endpoint string `yaml:"endpoint"`
	// Deployment is the Azure OpenAI deployment name.
	Deployment string `yaml:"deployment"`
	// APIVersion is the Azure OpenAI API version.
	APIVersion string `yaml:"api_version"`
}

// BedrockConfig holds AWS Bedrock provider settings.
type BedrockConfig struct {
	// Region is the AWS region for Bedrock.
	Region string `yaml:"region"`
	// ModelID is the Bedrock model identifier.
	ModelID string `yaml:"model_id"`
}

// GeminiConfig holds Google Gemini provider settings.
type GeminiConfig struct {
	// APIKey is the Google API key. Prefer env var GOOGLE_API_KEY.
	APIKey string `yaml:"api_key"`
	// Model is the Gemini model name.
	Model string `yaml:"model"`
}

// EmbeddingConfig holds embedding provider settings for RAG.
type EmbeddingConfig struct {
	// Provider selects the embedding backend (ollama, openai, azure).
	Provider string `yaml:"provider"`
	// Model is the embedding model name.
	Model string `yaml:"model"`
	// Dimensions overrides the embedding vector size.
	Dimensions int `yaml:"dimensions"`
	// APIKey is the embedding API key. Prefer env var EMBEDDING_API_KEY.
	APIKey string `yaml:"api_key"`
	// Endpoint is the embedding API endpoint.
	Endpoint string `yaml:"endpoint"`
}

// QdrantConfig holds Qdrant vector store settings.
type QdrantConfig struct {
	// Host is the Qdrant server hostname.
	Host string `yaml:"host"`
	// Port is the Qdrant gRPC port.
	Port int `yaml:"port"`
	// Collection is the Qdrant collection name.
	Collection string `yaml:"collection"`
	// APIKey is the Qdrant API key. Prefer env var QDRANT_API_KEY.
	APIKey string `yaml:"api_key"`
	// TLS enables TLS for the Qdrant connection.
	TLS bool `yaml:"tls"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Host is the bind address.
	Host string `yaml:"host"`
	// Port is the TCP port.
	Port int `yaml:"port"`
	// APIKey is the Bearer token for API authentication. Prefer env var TFAI_API_KEY.
	APIKey string `yaml:"api_key"`
}

// LoggingConfig holds structured logging settings.
type LoggingConfig struct {
	// Level is the minimum log level: debug, info, warn, error.
	Level string `yaml:"level"`
	// Format is the log output format: json, text.
	Format string `yaml:"format"`
}

// HistoryConfig holds conversation history settings.
type HistoryConfig struct {
	// DBPath is the SQLite database path. Set to "disabled" to disable.
	DBPath string `yaml:"db_path"`
}

// TracingConfig holds Langfuse tracing settings.
type TracingConfig struct {
	// PublicKey is the Langfuse public key. Prefer env var LANGFUSE_PUBLIC_KEY.
	PublicKey string `yaml:"public_key"`
	// SecretKey is the Langfuse secret key. Prefer env var LANGFUSE_SECRET_KEY.
	SecretKey string `yaml:"secret_key"`
	// Host is the Langfuse API host.
	Host string `yaml:"host"`
}

// envMapping maps YAML config fields to their corresponding env var names.
// Only non-empty YAML values are applied; env vars always take precedence.
var envMapping = []struct {
	envKey string
	value  func(*Config) string
}{
	{"MODEL_PROVIDER", func(c *Config) string { return c.Model.Provider }},
	{"MODEL_MAX_TOKENS", func(c *Config) string { return intStr(c.Model.MaxTokens) }},
	{"MODEL_TEMPERATURE", func(c *Config) string { return float32Str(c.Model.Temperature) }},
	{"OLLAMA_HOST", func(c *Config) string { return c.Model.Ollama.Host }},
	{"OLLAMA_MODEL", func(c *Config) string { return c.Model.Ollama.Model }},
	{"OPENAI_API_KEY", func(c *Config) string { return c.Model.OpenAI.APIKey }},
	{"OPENAI_MODEL", func(c *Config) string { return c.Model.OpenAI.Model }},
	{"AZURE_OPENAI_API_KEY", func(c *Config) string { return c.Model.Azure.APIKey }},
	{"AZURE_OPENAI_ENDPOINT", func(c *Config) string { return c.Model.Azure.Endpoint }},
	{"AZURE_OPENAI_DEPLOYMENT", func(c *Config) string { return c.Model.Azure.Deployment }},
	{"AZURE_OPENAI_API_VERSION", func(c *Config) string { return c.Model.Azure.APIVersion }},
	{"AWS_REGION", func(c *Config) string { return c.Model.Bedrock.Region }},
	{"BEDROCK_MODEL_ID", func(c *Config) string { return c.Model.Bedrock.ModelID }},
	{"GOOGLE_API_KEY", func(c *Config) string { return c.Model.Gemini.APIKey }},
	{"GEMINI_MODEL", func(c *Config) string { return c.Model.Gemini.Model }},
	{"EMBEDDING_PROVIDER", func(c *Config) string { return c.Embedding.Provider }},
	{"EMBEDDING_MODEL", func(c *Config) string { return c.Embedding.Model }},
	{"EMBEDDING_DIMENSIONS", func(c *Config) string { return intStr(c.Embedding.Dimensions) }},
	{"EMBEDDING_API_KEY", func(c *Config) string { return c.Embedding.APIKey }},
	{"EMBEDDING_ENDPOINT", func(c *Config) string { return c.Embedding.Endpoint }},
	{"QDRANT_HOST", func(c *Config) string { return c.Qdrant.Host }},
	{"QDRANT_PORT", func(c *Config) string { return intStr(c.Qdrant.Port) }},
	{"QDRANT_COLLECTION", func(c *Config) string { return c.Qdrant.Collection }},
	{"QDRANT_API_KEY", func(c *Config) string { return c.Qdrant.APIKey }},
	{"QDRANT_TLS", func(c *Config) string { return boolStr(c.Qdrant.TLS) }},
	{"LOG_LEVEL", func(c *Config) string { return c.Logging.Level }},
	{"LOG_FORMAT", func(c *Config) string { return c.Logging.Format }},
	{"TFAI_HISTORY_DB", func(c *Config) string { return c.History.DBPath }},
	{"LANGFUSE_PUBLIC_KEY", func(c *Config) string { return c.Tracing.PublicKey }},
	{"LANGFUSE_SECRET_KEY", func(c *Config) string { return c.Tracing.SecretKey }},
	{"LANGFUSE_HOST", func(c *Config) string { return c.Tracing.Host }},
}

// validProviders enumerates accepted values for model.provider.
var validProviders = map[string]bool{
	"ollama": true, "openai": true, "azure": true,
	"bedrock": true, "gemini": true,
}

// validEmbeddingProviders enumerates accepted values for embedding.provider.
var validEmbeddingProviders = map[string]bool{
	"ollama": true, "openai": true, "azure": true,
}

// validLogLevels enumerates accepted values for logging.level.
var validLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "error": true,
}

// validLogFormats enumerates accepted values for logging.format.
var validLogFormats = map[string]bool{
	"json": true, "text": true,
}

// Validate checks that all fields in the config have valid values.
// It is called by Load() after parsing so callers get a clear error at startup
// rather than mysterious runtime failures.
func (c *Config) Validate() error {
	var errs []string

	// --- Model ---
	if c.Model.Provider != "" && !validProviders[c.Model.Provider] {
		errs = append(errs, fmt.Sprintf(
			"model.provider: invalid value %q (valid: ollama, openai, azure, bedrock, gemini)",
			c.Model.Provider))
	}
	if c.Model.MaxTokens < 0 {
		errs = append(errs, fmt.Sprintf("model.max_tokens: %d must be non-negative", c.Model.MaxTokens))
	}
	if c.Model.Temperature != 0 && (c.Model.Temperature < 0 || c.Model.Temperature > 1) {
		errs = append(errs, fmt.Sprintf(
			"model.temperature: %.2f is out of range [0.0, 1.0]", c.Model.Temperature))
	}

	// --- Embedding ---
	if c.Embedding.Provider != "" && !validEmbeddingProviders[c.Embedding.Provider] {
		errs = append(errs, fmt.Sprintf(
			"embedding.provider: invalid value %q (valid: ollama, openai, azure)",
			c.Embedding.Provider))
	}
	if c.Embedding.Dimensions < 0 {
		errs = append(errs, fmt.Sprintf("embedding.dimensions: %d must be non-negative", c.Embedding.Dimensions))
	}

	// --- Server ---
	if c.Server.Port != 0 && (c.Server.Port < 1 || c.Server.Port > 65535) {
		errs = append(errs, fmt.Sprintf("server.port: %d is out of range [1, 65535]", c.Server.Port))
	}

	// --- Qdrant ---
	if c.Qdrant.Port != 0 && (c.Qdrant.Port < 1 || c.Qdrant.Port > 65535) {
		errs = append(errs, fmt.Sprintf("qdrant.port: %d is out of range [1, 65535]", c.Qdrant.Port))
	}

	// --- Logging ---
	if c.Logging.Level != "" && !validLogLevels[c.Logging.Level] {
		errs = append(errs, fmt.Sprintf(
			"logging.level: invalid value %q (valid: debug, info, warn, error)",
			c.Logging.Level))
	}
	if c.Logging.Format != "" && !validLogFormats[c.Logging.Format] {
		errs = append(errs, fmt.Sprintf(
			"logging.format: invalid value %q (valid: json, text)",
			c.Logging.Format))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config: validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// mu guards the package-level loaded config.
var (
	mu     sync.RWMutex
	loaded *Config
)

// Get returns the most recently loaded Config, or nil if Load has not been
// called (i.e. running in env-var-only mode).
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return loaded
}

// Load reads a YAML config file and applies non-empty values as environment
// variables. Existing env vars are never overwritten (env always wins).
// ${VAR} and $VAR references in YAML values are expanded from the process
// environment before parsing (CFG-10).
// Returns the path that was loaded, or empty string if no file was found.
func Load(explicitPath string, log *slog.Logger) (string, error) {
	path := resolveConfigPath(explicitPath)
	if path == "" {
		log.Debug("config: no YAML config file found, using env vars only")
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("config: failed to read %s: %w", path, err)
	}

	// Expand ${VAR} / $VAR references so secrets can be kept in the
	// environment while referenced from the YAML file (CFG-10).
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return "", fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	// Validate before applying — fail fast on bad config (CFG-8).
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("config: %s: %w", path, err)
	}

	applied := 0
	for _, m := range envMapping {
		yamlVal := m.value(&cfg)
		if yamlVal == "" || yamlVal == "0" || yamlVal == "false" {
			continue
		}
		if os.Getenv(m.envKey) != "" {
			continue // env var already set — do not override
		}
		_ = os.Setenv(m.envKey, yamlVal)
		applied++
	}

	// Store parsed config for structured access (CFG-9).
	mu.Lock()
	loaded = &cfg
	mu.Unlock()

	log.Info("config: loaded YAML config",
		slog.String("path", path),
		slog.Int("keys_applied", applied),
	)

	return path, nil
}

// resolveConfigPath returns the first config file path that exists.
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
		return ""
	}

	if envPath := os.Getenv("TFAI_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".tfai", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if _, err := os.Stat("tfai.yaml"); err == nil {
		return "tfai.yaml"
	}

	return ""
}

// intStr converts an int to string, returning "" for zero values.
func intStr(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}

// float32Str converts a float32 to string, returning "" for zero values.
func float32Str(v float32) string {
	if v == 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", v), "0"), ".")
}

// boolStr converts a bool to string, returning "" for false.
func boolStr(v bool) string {
	if !v {
		return ""
	}
	return "true"
}
