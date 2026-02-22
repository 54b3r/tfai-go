package config

import (
	"log/slog"
	"os"
	"path/filepath"
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
		os.Unsetenv(k)
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
