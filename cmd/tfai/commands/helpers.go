package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/qdrant/go-client/qdrant"

	"github.com/54b3r/tfai-go/internal/embedder"
	"github.com/54b3r/tfai-go/internal/provider"
	"github.com/54b3r/tfai-go/internal/rag"
	"github.com/54b3r/tfai-go/internal/server"
	tftools "github.com/54b3r/tfai-go/internal/tools"
)

// buildPingers constructs the readiness probes for GET /api/ready.
// The LLM pinger is always included and uses a zero-cost HTTP health check
// when the provider supports it, falling back to a Generate call otherwise.
// A Qdrant pinger is added when QDRANT_HOST is set in the environment.
func buildPingers(_ context.Context, chatModel model.ToolCallingChatModel, cfg *provider.Config, log *slog.Logger) []server.Pinger {
	hc := provider.NewHealthCheckConfig(cfg.Backend, cfg)

	pingers := []server.Pinger{
		server.NewLLMPinger(chatModel, hc, string(cfg.Backend)),
	}

	qdrantHost := os.Getenv("QDRANT_HOST")
	if qdrantHost != "" {
		client, err := qdrant.NewClient(&qdrant.Config{
			Host: qdrantHost,
			Port: getEnvInt("QDRANT_PORT", 6334),
		})
		if err != nil || client == nil {
			log.Warn("readiness: failed to create qdrant client, skipping probe",
				slog.String("host", qdrantHost),
				slog.Any("error", err),
			)
		} else {
			pingers = append(pingers, server.NewQdrantPinger(client))
			log.Info("readiness: qdrant probe registered",
				slog.String("host", qdrantHost),
				slog.Int("port", getEnvInt("QDRANT_PORT", 6334)),
			)
		}
	}

	return pingers
}

// buildRetriever constructs a rag.Retriever when QDRANT_HOST is set in the
// environment. Returns (nil, noop, nil) when Qdrant is not configured — the
// agent treats a nil retriever as "RAG disabled". Returns a non-nil error when
// QDRANT_HOST is set but the embedder configuration is invalid, so callers can
// fail fast with a clear message. The returned closer must be called (e.g. via
// defer) to release the underlying gRPC connection.
func buildRetriever(ctx context.Context, log *slog.Logger) (rag.Retriever, func(), error) {
	noop := func() {}

	qdrantHost := os.Getenv("QDRANT_HOST")
	if qdrantHost == "" {
		return nil, noop, nil
	}

	if err := embedder.ValidateForRAG(log); err != nil {
		return nil, noop, err
	}

	emb, err := embedder.NewFromEnv()
	if err != nil {
		return nil, noop, fmt.Errorf("rag: failed to initialise embedder: %w", err)
	}

	qdrantPort := getEnvInt("QDRANT_PORT", 6334)
	collection := getEnvOrDefault("QDRANT_COLLECTION", "tfai-docs")
	vectorSize := uint64(embedder.DefaultDimensions(getEnvOrDefault("EMBEDDING_PROVIDER", getEnvOrDefault("MODEL_PROVIDER", "ollama")))) //nolint:gosec // dimensions are bounded

	qstore, err := rag.NewQdrantStore(ctx, &rag.QdrantConfig{
		Host:       qdrantHost,
		Port:       qdrantPort,
		Collection: collection,
		VectorSize: vectorSize,
		APIKey:     os.Getenv("QDRANT_API_KEY"),
		UseTLS:     os.Getenv("QDRANT_TLS") == "true",
	})
	if err != nil {
		return nil, noop, fmt.Errorf("rag: failed to connect to Qdrant at %s:%d: %w", qdrantHost, qdrantPort, err)
	}

	retriever, err := rag.NewRetriever(emb, qstore, getEnvInt("RAG_TOP_K", 5))
	if err != nil {
		_ = qstore.Close()
		return nil, noop, fmt.Errorf("rag: failed to create retriever: %w", err)
	}

	log.Info("rag: retriever ready",
		slog.String("host", qdrantHost),
		slog.Int("port", qdrantPort),
		slog.String("collection", collection),
	)
	return retriever, func() { _ = qstore.Close() }, nil
}

// buildTools constructs the full list of Eino-compatible Terraform tools to
// register with the agent. If runner is nil, tools that require a live
// terraform binary are omitted gracefully.
//
// Note: terraform_generate is intentionally excluded. File generation is
// handled by parseAgentOutput + applyFiles in agent.Query(), which parses
// the JSON envelope from the LLM's text response directly.
func buildTools(runner tftools.Runner) []tool.BaseTool {
	var toolList []tool.BaseTool

	// plan and state tools require a live terraform binary.
	if runner != nil {
		toolList = append(toolList,
			tftools.NewPlanTool(runner),
			tftools.NewStateTool(runner),
		)
	}

	return toolList
}

// getEnvOrDefault returns the value of the named environment variable, or
// fallback if the variable is unset or empty.
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns the integer value of the named environment variable, or
// fallback if the variable is unset, empty, or not parseable as an integer.
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
