//go:build integration

package embedder

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestOllamaEmbedder_Integration performs a real HTTP call to a locally running
// Ollama instance to validate the embedder end-to-end.
//
// Prerequisites:
//
//	ollama pull nomic-embed-text
//	ollama serve   (or it must already be running)
//
// Run with:
//
//	go test -tags=integration -run TestOllamaEmbedder_Integration ./internal/embedder/
//
// In CI, set OLLAMA_HOST if Ollama is not on localhost:11434.
func TestOllamaEmbedder_Integration(t *testing.T) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = "nomic-embed-text"
	}

	emb := NewOllamaEmbedder(&OllamaConfig{
		Host:  host,
		Model: model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	texts := []string{
		"aws_s3_bucket is a Terraform resource for managing S3 buckets.",
		"aws_eks_cluster provisions an Amazon EKS Kubernetes cluster.",
	}

	embeddings, err := emb.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed() failed: %v\n\nEnsure Ollama is running and %q is pulled:\n  ollama pull %s", err, model, model)
	}

	if len(embeddings) != len(texts) {
		t.Fatalf("expected %d embeddings, got %d", len(texts), len(embeddings))
	}

	for i, vec := range embeddings {
		if len(vec) == 0 {
			t.Errorf("embedding[%d] is empty", i)
		}
		t.Logf("embedding[%d]: dim=%d, first_3=%v", i, len(vec), vec[:3])
	}

	// Validate that the two embeddings are distinct (not identical vectors).
	if len(embeddings[0]) == len(embeddings[1]) {
		identical := true
		for j := range embeddings[0] {
			if embeddings[0][j] != embeddings[1][j] {
				identical = false
				break
			}
		}
		if identical {
			t.Error("embeddings[0] and embeddings[1] are identical — model may not be working correctly")
		}
	}

	// Log the dimension so the caller can confirm it matches their Qdrant collection.
	t.Logf("model=%s dim=%d (set EMBEDDING_DIMENSIONS=%d for Qdrant collection)", model, len(embeddings[0]), len(embeddings[0]))
}
