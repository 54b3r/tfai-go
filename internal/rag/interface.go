// Package rag defines the interfaces for retrieval-augmented generation
// components: vector storage, document retrieval, and embedding.
// Concrete implementations (Qdrant, etc.) satisfy these interfaces so the
// agent layer never depends on a specific backend.
package rag

import (
	"context"
)

// Document represents a unit of retrieved or stored knowledge.
type Document struct {
	// ID is the unique identifier for this document chunk.
	ID string

	// Content is the raw text content of the chunk.
	Content string

	// Source is the origin URI or file path of the document.
	Source string

	// Metadata holds arbitrary key-value pairs (provider, resource type, etc.).
	Metadata map[string]string

	// Score is the similarity score assigned during retrieval (0.0–1.0).
	// Zero value means the score was not computed.
	Score float32
}

// VectorStore is the interface for persisting and searching document embeddings.
// Implementations must be safe to call from multiple goroutines.
type VectorStore interface {
	// Upsert stores or updates a batch of documents with their pre-computed embeddings.
	// The embeddings slice must be parallel to docs — embeddings[i] is the vector for docs[i].
	Upsert(ctx context.Context, docs []Document, embeddings [][]float32) error

	// Search performs a semantic similarity search and returns the top-k
	// most relevant documents for the given query embedding.
	Search(ctx context.Context, queryEmbedding []float32, topK int) ([]Document, error)

	// Delete removes documents by their IDs.
	Delete(ctx context.Context, ids []string) error

	// Close releases any resources held by the store.
	Close() error
}

// Embedder is the interface for converting text into dense vector embeddings.
// Implementations must be safe to call from multiple goroutines.
type Embedder interface {
	// Embed converts a batch of texts into their corresponding embeddings.
	// The returned slice is parallel to the input slice.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Retriever is the high-level interface used by the agent to fetch relevant
// context for a given query. It combines embedding and vector search.
// Implementations must be safe to call from multiple goroutines.
type Retriever interface {
	// Retrieve returns the top-k most relevant documents for the given query.
	Retrieve(ctx context.Context, query string, topK int) ([]Document, error)
}
