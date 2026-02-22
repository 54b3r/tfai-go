// Package embedder provides implementations of the rag.Embedder interface for
// converting text into dense vector embeddings. Each implementation talks to a
// different backend (OpenAI, Azure OpenAI, Ollama) via plain HTTP — no
// additional SDK dependencies are required.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIEmbedder implements rag.Embedder using the OpenAI (or Azure OpenAI)
// embeddings REST API. It is safe for concurrent use.
type OpenAIEmbedder struct {
	// baseURL is the API base (e.g. "https://api.openai.com/v1" or an Azure endpoint).
	baseURL string
	// apiKey is the Bearer token (OpenAI) or api-key header value (Azure).
	apiKey string
	// model is the embedding model name (e.g. "text-embedding-3-small").
	model string
	// dimensions is the desired embedding vector length (0 = model default).
	dimensions int
	// azure selects Azure-style auth (api-key header) over Bearer token.
	azure bool
	// apiVersion is the Azure OpenAI API version query param (ignored for OpenAI).
	apiVersion string
	// client is the shared HTTP client with a sensible timeout.
	client *http.Client
}

// OpenAIConfig holds the settings for constructing an OpenAIEmbedder.
type OpenAIConfig struct {
	// BaseURL is the API base URL. For OpenAI: "https://api.openai.com/v1".
	// For Azure: "https://<resource>.openai.azure.com/openai".
	BaseURL string
	// APIKey is the authentication key.
	APIKey string
	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string
	// Dimensions is the desired vector length (0 = model default).
	Dimensions int
	// Azure enables Azure OpenAI mode (api-key header + api-version param).
	Azure bool
	// APIVersion is the Azure OpenAI API version (e.g. "2025-04-01-preview").
	// Ignored when Azure is false.
	APIVersion string
}

// NewOpenAIEmbedder constructs an OpenAIEmbedder from the given config.
func NewOpenAIEmbedder(cfg *OpenAIConfig) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		azure:      cfg.Azure,
		apiVersion: cfg.APIVersion,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// openaiEmbedRequest is the JSON body sent to the embeddings endpoint.
type openaiEmbedRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// openaiEmbedResponse is the JSON body returned from the embeddings endpoint.
type openaiEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed converts a batch of texts into their corresponding embeddings.
// The returned slice is parallel to the input slice.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := openaiEmbedRequest{
		Input: texts,
		Model: e.model,
	}
	if e.dimensions > 0 {
		body.Dimensions = e.dimensions
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: marshal request: %w", err)
	}

	url := e.baseURL + "/embeddings"
	if e.azure {
		url = e.baseURL + "/deployments/" + e.model + "/embeddings?api-version=" + e.apiVersion
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.azure {
		req.Header.Set("api-key", e.apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: request failed: %w", err)
	}
	defer resp.Body.Close()

	var result openaiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai embedder: decode response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if result.Error != nil {
			msg = result.Error.Message
		}
		return nil, fmt.Errorf("openai embedder: %s", msg)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("openai embedder: expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	// The API may return data out of order; sort by index.
	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("openai embedder: index %d out of range [0, %d)", d.Index, len(texts))
		}
		embeddings[d.Index] = d.Embedding
	}

	return embeddings, nil
}
