package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaEmbedder implements rag.Embedder using the Ollama /api/embed endpoint.
// It is safe for concurrent use. No API key is required — Ollama runs locally.
type OllamaEmbedder struct {
	// host is the Ollama server base URL (e.g. "http://localhost:11434").
	host string
	// model is the embedding model name (e.g. "nomic-embed-text").
	model string
	// client is the shared HTTP client with a sensible timeout.
	client *http.Client
}

// OllamaConfig holds the settings for constructing an OllamaEmbedder.
type OllamaConfig struct {
	// Host is the Ollama server base URL (e.g. "http://localhost:11434").
	Host string
	// Model is the embedding model name (e.g. "nomic-embed-text").
	Model string
}

// NewOllamaEmbedder constructs an OllamaEmbedder from the given config.
func NewOllamaEmbedder(cfg *OllamaConfig) *OllamaEmbedder {
	return &OllamaEmbedder{
		host:   cfg.Host,
		model:  cfg.Model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// ollamaEmbedRequest is the JSON body sent to the Ollama /api/embed endpoint.
type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ollamaEmbedResponse is the JSON body returned from the Ollama /api/embed endpoint.
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// Embed converts a batch of texts into their corresponding embeddings.
// The returned slice is parallel to the input slice.
func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := ollamaEmbedRequest{
		Model: e.model,
		Input: texts,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: marshal request: %w", err)
	}

	url := e.host + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: request failed: %w", err)
	}
	defer resp.Body.Close()

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embedder: decode response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if result.Error != "" {
			msg = result.Error
		}
		return nil, fmt.Errorf("ollama embedder: %s", msg)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embedder: expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}

	return result.Embeddings, nil
}
