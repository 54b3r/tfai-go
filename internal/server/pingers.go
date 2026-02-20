package server

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/qdrant/go-client/qdrant"
)

// LLMPinger probes an LLM backend by sending a minimal single-token generate
// request. It satisfies the Pinger interface and is used by GET /api/ready.
type LLMPinger struct {
	// model is the chat model to probe.
	model model.ToolCallingChatModel
	// name identifies the backend in readiness responses (e.g. "ollama").
	name string
}

// NewLLMPinger constructs an LLMPinger for the given model and backend name.
func NewLLMPinger(m model.ToolCallingChatModel, name string) *LLMPinger {
	return &LLMPinger{model: m, name: name}
}

// Name returns the backend label used in readiness responses.
func (p *LLMPinger) Name() string { return p.name }

// Ping sends a minimal generate request to the LLM backend.
// Returns nil if the backend responds, or an error if it is unreachable or
// returns an unexpected failure. The context deadline controls the timeout.
func (p *LLMPinger) Ping(ctx context.Context) error {
	msgs := []*schema.Message{
		schema.UserMessage("ping"),
	}
	resp, err := p.model.Generate(ctx, msgs)
	if err != nil {
		return fmt.Errorf("generate failed: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("generate returned nil response")
	}
	return nil
}

// QdrantPinger probes a Qdrant instance using its native HealthCheck RPC.
// It satisfies the Pinger interface and is used by GET /api/ready.
type QdrantPinger struct {
	// client is the Qdrant gRPC client to probe.
	client *qdrant.Client
}

// NewQdrantPinger constructs a QdrantPinger for the given Qdrant client.
func NewQdrantPinger(client *qdrant.Client) *QdrantPinger {
	return &QdrantPinger{client: client}
}

// Name returns the dependency label used in readiness responses.
func (p *QdrantPinger) Name() string { return "qdrant" }

// Ping calls the Qdrant HealthCheck RPC.
// Returns nil if Qdrant is reachable, or a descriptive error otherwise.
func (p *QdrantPinger) Ping(ctx context.Context) error {
	_, err := p.client.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}
