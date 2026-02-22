package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/qdrant/go-client/qdrant"

	"github.com/54b3r/tfai-go/internal/provider"
)

// LLMPinger probes an LLM backend by sending a minimal single-token generate
// request. It satisfies the Pinger interface and is used by GET /api/ready.
type LLMPinger struct {
	// DEPRECATED: model is the chat model to probe.
	model model.ToolCallingChatModel
	// Use healthCheck httpCheckers instead to save on LLM calls (token waste)
	healthCheck provider.HealthCheckConfig
	// name identifies the backend in readiness responses (e.g. "ollama").
	name string
}

// NewLLMPinger constructs an LLMPinger for the given model and backend name.
// TODO: Remove model parameter when all providers are migrated to use healthCheck
func NewLLMPinger(m model.ToolCallingChatModel, hc provider.HealthCheckConfig, name string) *LLMPinger {
	return &LLMPinger{model: m, healthCheck: hc, name: name}
}

// Name returns the backend label used in readiness responses.
func (p *LLMPinger) Name() string { return p.name }

// Ping probes the LLM backend for readiness. When a zero-cost HealthCheckConfig
// is available it is used exclusively; otherwise it falls back to a single-token
// Generate call (which consumes tokens — avoid where possible).
func (p *LLMPinger) Ping(ctx context.Context) error {
	if p.healthCheck != nil {
		if err := p.healthCheck.HealthCheck(ctx); err != nil {
			return fmt.Errorf("%s health check failed: %w", p.name, err)
		}
		return nil
	}

	// Legacy fallback — burns tokens. Remove when all providers implement HealthCheckConfig.
	slog.Warn("pinger: falling back to Generate-based health check — tokens will be consumed",
		slog.String("backend", p.name),
	)
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
