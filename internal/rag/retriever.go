package rag

import (
	"context"
	"fmt"
)

// DefaultRetriever implements the Retriever interface by combining an Embedder
// and a VectorStore. It embeds the query at retrieval time and delegates
// similarity search to the store.
type DefaultRetriever struct {
	// embedder converts query text to a dense vector.
	embedder Embedder

	// store performs the vector similarity search.
	store VectorStore

	// defaultTopK is the number of results to return when the caller passes 0.
	defaultTopK int
}

// NewRetriever constructs a DefaultRetriever from the given Embedder and VectorStore.
// defaultTopK sets the fallback result count when Retrieve is called with topK=0.
func NewRetriever(embedder Embedder, store VectorStore, defaultTopK int) (*DefaultRetriever, error) {
	if embedder == nil {
		return nil, fmt.Errorf("rag: embedder must not be nil")
	}
	if store == nil {
		return nil, fmt.Errorf("rag: store must not be nil")
	}
	if defaultTopK <= 0 {
		defaultTopK = 5
	}
	return &DefaultRetriever{
		embedder:    embedder,
		store:       store,
		defaultTopK: defaultTopK,
	}, nil
}

// Retrieve embeds the query and returns the top-k most relevant documents.
// If topK is 0 the defaultTopK configured at construction time is used.
func (r *DefaultRetriever) Retrieve(ctx context.Context, query string, topK int) ([]Document, error) {
	if topK <= 0 {
		topK = r.defaultTopK
	}

	embeddings, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("rag: embedding query failed: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("rag: embedder returned empty result for query")
	}

	docs, err := r.store.Search(ctx, embeddings[0], topK)
	if err != nil {
		return nil, fmt.Errorf("rag: vector search failed: %w", err)
	}

	return docs, nil
}
