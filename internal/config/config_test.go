package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_NoFile(t *testing.T) {
	t.Parallel()

	log := slog.Default()
	path, err := Load("/nonexistent/path/config.yaml", log)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := []byte(`
model:
  provider: azure
  max_tokens: 8192
  temperature: 0.3
  azure:
    endpoint: https://my-resource.openai.azure.com
    deployment: gpt-4o
    api_version: "2025-04-01-preview"
embedding:
  provider: ollama
  model: nomic-embed-text
qdrant:
  host: qdrant.internal
  port: 6334
  collection: my-docs
logging:
  level: debug
  format: text
`)

	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Clear env vars that the YAML should set.
	envKeys := []string{
		"MODEL_PROVIDER", "MODEL_MAX_TOKENS", "MODEL_TEMPERATURE",
		"AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_API_VERSION",
		"EMBEDDING_PROVIDER", "EMBEDDING_MODEL",
		"QDRANT_HOST", "QDRANT_PORT", "QDRANT_COLLECTION",
		"LOG_LEVEL", "LOG_FORMAT",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	log := slog.Default()
	loaded, err := Load(cfgPath, log)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded != cfgPath {
		t.Errorf("loaded path: got %q, want %q", loaded, cfgPath)
	}

	checks := map[string]string{
		"MODEL_PROVIDER":           "azure",
		"MODEL_MAX_TOKENS":         "8192",
		"AZURE_OPENAI_ENDPOINT":    "https://my-resource.openai.azure.com",
		"AZURE_OPENAI_DEPLOYMENT":  "gpt-4o",
		"AZURE_OPENAI_API_VERSION": "2025-04-01-preview",
		"EMBEDDING_PROVIDER":       "ollama",
		"EMBEDDING_MODEL":          "nomic-embed-text",
		"QDRANT_HOST":              "qdrant.internal",
		"QDRANT_PORT":              "6334",
		"QDRANT_COLLECTION":        "my-docs",
		"LOG_LEVEL":                "debug",
		"LOG_FORMAT":               "text",
	}
	for k, want := range checks {
		got := os.Getenv(k)
		if got != want {
			t.Errorf("%s: got %q, want %q", k, got, want)
		}
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := []byte(`
model:
  provider: ollama
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Set env var BEFORE loading — it should NOT be overwritten.
	t.Setenv("MODEL_PROVIDER", "azure")

	log := slog.Default()
	_, err := Load(cfgPath, log)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := os.Getenv("MODEL_PROVIDER"); got != "azure" {
		t.Errorf("MODEL_PROVIDER: expected env override %q, got %q", "azure", got)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	log := slog.Default()
	_, err := Load(cfgPath, log)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- CFG-8: Validation tests (CFG-T1 through CFG-T6) ---

func TestValidate_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Model: ModelConfig{
			Provider:    "azure",
			MaxTokens:   4096,
			Temperature: 0.3,
		},
		Embedding: EmbeddingConfig{Provider: "openai"},
		Server:    ServerConfig{Port: 8080},
		Qdrant:    QdrantConfig{Port: 6334},
		Logging:   LoggingConfig{Level: "info", Format: "json"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	t.Parallel()
	cfg := &Config{Model: ModelConfig{Provider: "invalid-backend"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid provider")
	}
	if !strings.Contains(err.Error(), "model.provider") {
		t.Errorf("error should mention model.provider, got: %v", err)
	}
}

func TestValidate_InvalidEmbeddingProvider(t *testing.T) {
	t.Parallel()
	cfg := &Config{Embedding: EmbeddingConfig{Provider: "bedrock"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid embedding provider")
	}
	if !strings.Contains(err.Error(), "embedding.provider") {
		t.Errorf("error should mention embedding.provider, got: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	t.Parallel()
	cfg := &Config{Server: ServerConfig{Port: 99999}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for out-of-range port")
	}
	if !strings.Contains(err.Error(), "server.port") {
		t.Errorf("error should mention server.port, got: %v", err)
	}
}

func TestValidate_InvalidQdrantPort(t *testing.T) {
	t.Parallel()
	cfg := &Config{Qdrant: QdrantConfig{Port: -1}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for negative qdrant port")
	}
	if !strings.Contains(err.Error(), "qdrant.port") {
		t.Errorf("error should mention qdrant.port, got: %v", err)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	t.Parallel()
	cfg := &Config{Logging: LoggingConfig{Level: "verbose"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid log level")
	}
	if !strings.Contains(err.Error(), "logging.level") {
		t.Errorf("error should mention logging.level, got: %v", err)
	}
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	t.Parallel()
	cfg := &Config{Logging: LoggingConfig{Format: "xml"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid log format")
	}
	if !strings.Contains(err.Error(), "logging.format") {
		t.Errorf("error should mention logging.format, got: %v", err)
	}
}

func TestValidate_TemperatureOutOfRange(t *testing.T) {
	t.Parallel()
	cfg := &Config{Model: ModelConfig{Temperature: 1.5}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for temperature > 1.0")
	}
	if !strings.Contains(err.Error(), "model.temperature") {
		t.Errorf("error should mention model.temperature, got: %v", err)
	}
}

func TestValidate_NegativeMaxTokens(t *testing.T) {
	t.Parallel()
	cfg := &Config{Model: ModelConfig{MaxTokens: -100}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for negative max_tokens")
	}
	if !strings.Contains(err.Error(), "model.max_tokens") {
		t.Errorf("error should mention model.max_tokens, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Model:   ModelConfig{Provider: "bad", MaxTokens: -1},
		Logging: LoggingConfig{Level: "verbose"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	// Should collect all errors, not just the first.
	if !strings.Contains(err.Error(), "model.provider") {
		t.Errorf("error should mention model.provider")
	}
	if !strings.Contains(err.Error(), "model.max_tokens") {
		t.Errorf("error should mention model.max_tokens")
	}
	if !strings.Contains(err.Error(), "logging.level") {
		t.Errorf("error should mention logging.level")
	}
}

func TestValidate_ZeroValuesPass(t *testing.T) {
	t.Parallel()
	// All-zero config should pass — zero means "not set".
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero-value config should pass validation, got: %v", err)
	}
}

// --- CFG-10: Environment variable interpolation ---

func TestLoad_EnvVarInterpolation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// YAML references an env var via ${...} syntax.
	content := []byte(`
model:
  provider: openai
  openai:
    api_key: ${TEST_INTERPOLATION_KEY}
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Set the env var that the YAML references.
	t.Setenv("TEST_INTERPOLATION_KEY", "sk-secret-123")
	// Clear the target env var so Load() actually sets it.
	_ = os.Unsetenv("OPENAI_API_KEY")
	_ = os.Unsetenv("MODEL_PROVIDER")

	log := slog.Default()
	_, err := Load(cfgPath, log)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := os.Getenv("OPENAI_API_KEY"); got != "sk-secret-123" {
		t.Errorf("OPENAI_API_KEY: got %q, want %q", got, "sk-secret-123")
	}
}

// --- CFG-9: config.Get() accessor ---

func TestGet_ReturnsLoadedConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := []byte(`
model:
  provider: ollama
logging:
  level: debug
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Clear env vars.
	_ = os.Unsetenv("MODEL_PROVIDER")
	_ = os.Unsetenv("LOG_LEVEL")

	log := slog.Default()
	_, err := Load(cfgPath, log)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil after successful Load")
	}
	if cfg.Model.Provider != "ollama" {
		t.Errorf("Model.Provider: got %q, want %q", cfg.Model.Provider, "ollama")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level: got %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestLoad_ValidationRejectsInvalidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Invalid provider should be caught during Load.
	content := []byte(`
model:
  provider: not-a-provider
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	log := slog.Default()
	_, err := Load(cfgPath, log)
	if err == nil {
		t.Fatal("expected Load to fail on invalid provider")
	}
	if !strings.Contains(err.Error(), "model.provider") {
		t.Errorf("error should mention model.provider, got: %v", err)
	}
}

func TestFloat32Str(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   float32
		want string
	}{
		{0.0, ""},
		{0.2, "0.2"},
		{0.3, "0.3"},
		{1.0, "1"},
	}
	for _, tt := range tests {
		if got := float32Str(tt.in); got != tt.want {
			t.Errorf("float32Str(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
