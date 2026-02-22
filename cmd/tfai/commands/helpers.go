package commands

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/qdrant/go-client/qdrant"

	"github.com/54b3r/tfai-go/internal/provider"
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
			Port: 6334,
		})
		if err != nil {
			log.Warn("readiness: failed to create qdrant client, skipping probe",
				slog.String("host", qdrantHost),
				slog.Any("error", err),
			)
		} else {
			pingers = append(pingers, server.NewQdrantPinger(client))
		}
	}

	return pingers
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
