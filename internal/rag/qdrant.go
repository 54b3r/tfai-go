package rag

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

// QdrantConfig holds connection parameters for a Qdrant vector store instance.
type QdrantConfig struct {
	// Host is the Qdrant server hostname (default: localhost).
	Host string

	// Port is the Qdrant gRPC port (default: 6334).
	Port int

	// Collection is the Qdrant collection name to use.
	Collection string

	// VectorSize is the dimensionality of the embeddings stored in this collection.
	VectorSize uint64

	// APIKey is the optional Qdrant API key for authenticated clusters.
	APIKey string

	// UseTLS enables TLS for the gRPC connection.
	UseTLS bool
}

// QdrantStore implements VectorStore backed by a Qdrant instance.
type QdrantStore struct {
	// client is the underlying Qdrant gRPC client.
	client *qdrant.Client

	// cfg holds the resolved configuration for this store.
	cfg *QdrantConfig
}

// NewQdrantStore creates a new QdrantStore, ensuring the target collection
// exists (creating it if necessary), and returns a ready-to-use VectorStore.
func NewQdrantStore(ctx context.Context, cfg *QdrantConfig) (*QdrantStore, error) {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6334
	}

	clientCfg := &qdrant.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
		UseTLS: cfg.UseTLS,
	}

	client, err := qdrant.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to create client: %w", err)
	}

	store := &QdrantStore{client: client, cfg: cfg}
	if err := store.ensureCollection(ctx); err != nil {
		return nil, err
	}

	return store, nil
}

// ensureCollection creates the Qdrant collection if it does not already exist.
func (s *QdrantStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.CollectionExists(ctx, s.cfg.Collection)
	if err != nil {
		return fmt.Errorf("qdrant: failed to check collection existence: %w", err)
	}
	if exists {
		return nil
	}

	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: s.cfg.Collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     s.cfg.VectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to create collection %q: %w", s.cfg.Collection, err)
	}

	return nil
}

// Upsert stores or updates a batch of documents with their embeddings.
// Each Document must have its embedding pre-computed; this method does not
// call the Embedder — use the Retriever implementation for end-to-end upsert.
func (s *QdrantStore) Upsert(ctx context.Context, docs []Document) error {
	points := make([]*qdrant.PointStruct, 0, len(docs))
	for _, doc := range docs {
		payload := map[string]interface{}{
			"content": doc.Content,
			"source":  doc.Source,
		}
		for k, v := range doc.Metadata {
			payload[k] = v
		}

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(doc.ID),
			Payload: qdrant.NewValueMap(payload),
		})
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.cfg.Collection,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("qdrant: upsert failed: %w", err)
	}

	return nil
}

// Search performs a cosine similarity search and returns the top-k results.
func (s *QdrantStore) Search(ctx context.Context, queryEmbedding []float32, topK int) ([]Document, error) {
	limit := uint64(topK)
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.cfg.Collection,
		Query:          qdrant.NewQuery(queryEmbedding...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: search failed: %w", err)
	}

	docs := make([]Document, 0, len(results))
	for _, r := range results {
		doc := Document{
			ID:       r.Id.GetUuid(),
			Score:    r.Score,
			Metadata: make(map[string]string),
		}
		if p := r.Payload; p != nil {
			if v, ok := p["content"]; ok {
				doc.Content = v.GetStringValue()
			}
			if v, ok := p["source"]; ok {
				doc.Source = v.GetStringValue()
			}
			for k, v := range p {
				if k != "content" && k != "source" {
					doc.Metadata[k] = v.GetStringValue()
				}
			}
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// Delete removes documents from the collection by their IDs.
func (s *QdrantStore) Delete(ctx context.Context, ids []string) error {
	pointIDs := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		pointIDs = append(pointIDs, qdrant.NewIDUUID(id))
	}

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.cfg.Collection,
		Points:         qdrant.NewPointsSelector(pointIDs...),
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete failed: %w", err)
	}

	return nil
}

// Close closes the underlying Qdrant gRPC connection.
func (s *QdrantStore) Close() error {
	return s.client.Close()
}
