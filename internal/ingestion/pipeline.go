// Package ingestion implements the documentation ingestion pipeline.
// It fetches Terraform provider documentation pages, chunks the content,
// embeds each chunk, and upserts the results into the vector store.
// This pipeline is invoked by the `tfai ingest` CLI command.
package ingestion

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/54b3r/tfai-go/internal/rag"
)

// Source describes a documentation source to be ingested.
type Source struct {
	// URL is the HTTP(S) URL of the documentation page to fetch.
	URL string

	// Provider identifies the cloud provider (aws, azure, gcp, generic).
	Provider string

	// ResourceType is the Terraform resource type this doc covers (e.g. "aws_eks_cluster").
	ResourceType string
}

// Config holds the configuration for the ingestion pipeline.
type Config struct {
	// ChunkSize is the maximum number of characters per document chunk.
	// Defaults to 1000 if zero.
	ChunkSize int

	// ChunkOverlap is the number of characters to overlap between consecutive chunks.
	// Defaults to 100 if zero.
	ChunkOverlap int

	// HTTPTimeout is the timeout for each documentation fetch request.
	// Defaults to 30s if zero.
	HTTPTimeout time.Duration

	// UserAgent is the HTTP User-Agent header sent with fetch requests.
	UserAgent string
}

// Pipeline orchestrates the fetch → chunk → embed → upsert flow for a set
// of documentation sources.
type Pipeline struct {
	// embedder converts text chunks into dense vector embeddings.
	embedder rag.Embedder

	// store persists the embedded chunks.
	store rag.VectorStore

	// cfg holds the resolved pipeline configuration.
	cfg *Config

	// httpClient is the HTTP client used for fetching documentation pages.
	httpClient *http.Client
}

// NewPipeline constructs a Pipeline from the provided dependencies and config.
func NewPipeline(embedder rag.Embedder, store rag.VectorStore, cfg *Config) (*Pipeline, error) {
	if embedder == nil {
		return nil, fmt.Errorf("ingestion: embedder must not be nil")
	}
	if store == nil {
		return nil, fmt.Errorf("ingestion: store must not be nil")
	}
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1000
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 0
	}
	if cfg.ChunkOverlap >= cfg.ChunkSize {
		cfg.ChunkOverlap = cfg.ChunkSize / 10
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "tfai-go/1.0 (terraform documentation ingestion)"
	}

	return &Pipeline{
		embedder: embedder,
		store:    store,
		cfg:      cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}, nil
}

// Ingest fetches, chunks, embeds, and stores all provided sources.
// It processes sources sequentially and returns the first error encountered.
// Progress is reported via the optional progress callback.
func (p *Pipeline) Ingest(ctx context.Context, sources []Source, progress func(msg string)) error {
	if progress == nil {
		progress = func(string) {}
	}

	for _, src := range sources {
		progress(fmt.Sprintf("fetching %s", src.URL))

		content, err := p.fetch(ctx, src.URL)
		if err != nil {
			return fmt.Errorf("ingestion: fetch failed for %s: %w", src.URL, err)
		}

		chunks := p.chunk(content)
		progress(fmt.Sprintf("chunked %s into %d chunks", src.URL, len(chunks)))

		texts := make([]string, len(chunks))
		copy(texts, chunks)

		embeddings, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("ingestion: embedding failed for %s: %w", src.URL, err)
		}

		docs := make([]rag.Document, 0, len(chunks))
		for i, chunk := range chunks {
			id := chunkID(src.URL, i)
			doc := rag.Document{
				ID:      id,
				Content: chunk,
				Source:  src.URL,
				Metadata: map[string]string{
					"provider":      src.Provider,
					"resource_type": src.ResourceType,
					"chunk_index":   fmt.Sprintf("%d", i),
				},
			}
			docs = append(docs, doc)
		}

		if err := p.store.Upsert(ctx, docs, embeddings); err != nil {
			return fmt.Errorf("ingestion: upsert failed for %s: %w", src.URL, err)
		}

		progress(fmt.Sprintf("ingested %d chunks from %s", len(chunks), src.URL))
	}

	return nil
}

// fetch retrieves the raw text content of a URL.
func (p *Pipeline) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Accept", "text/plain, text/html")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	return string(body), nil
}

// chunk splits text into overlapping chunks of cfg.ChunkSize characters.
func (p *Pipeline) chunk(text string) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}

	var chunks []string
	size := p.cfg.ChunkSize
	overlap := p.cfg.ChunkOverlap

	for start := 0; start < len(text); start += size - overlap {
		end := start + size
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[start:end])
		if end == len(text) {
			break
		}
	}

	return chunks
}

// chunkID generates a deterministic ID for a document chunk based on its
// source URL and chunk index.
func chunkID(sourceURL string, index int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", sourceURL, index)))
	return fmt.Sprintf("%x", h[:16])
}
