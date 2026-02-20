package commands

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino/callbacks"
	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/agent"
	"github.com/54b3r/tfai-go/internal/provider"
	"github.com/54b3r/tfai-go/internal/server"
	"github.com/54b3r/tfai-go/internal/tools"
	"github.com/54b3r/tfai-go/internal/tracing"
)

// NewServeCmd constructs the `tfai serve` command, which starts the HTTP
// server and serves the web UI for interactive use.
func NewServeCmd() *cobra.Command {
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the TF-AI HTTP server and web UI",
		Long: `Start the TF-AI HTTP server on localhost.

The server exposes a REST/SSE API and serves the web UI for interactive
Terraform assistance. The web UI provides a file workspace view and chat
interface similar to a local IDE companion.

Examples:
  tfai serve
  tfai serve --port 9090
  MODEL_PROVIDER=azure tfai serve`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log.Printf("serve: MODEL_PROVIDER=%q", os.Getenv("MODEL_PROVIDER"))

			// Setup Langfuse tracing — opt-in, no-op if keys are absent.
			handler, flush, ok := tracing.Setup()
			if ok {
				callbacks.AppendGlobalHandlers(handler)
				defer flush()
				log.Printf("serve: langfuse tracing enabled")
			} else {
				log.Printf("serve: langfuse tracing disabled (LANGFUSE_PUBLIC_KEY not set)")
			}

			chatModel, err := provider.NewFromEnv(ctx)
			if err != nil {
				return fmt.Errorf("serve: failed to initialise model provider: %w", err)
			}
			log.Printf("serve: provider initialised successfully")

			runner, err := tools.NewExecRunner()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v (plan/state tools unavailable)\n", err)
				runner = nil
			}

			agentTools := buildTools(runner)

			tfAgent, err := agent.New(ctx, &agent.Config{
				ChatModel: chatModel,
				Tools:     agentTools,
			})
			if err != nil {
				return fmt.Errorf("serve: failed to initialise agent: %w", err)
			}

			srv, err := server.New(tfAgent, &server.Config{
				Host: host,
				Port: port,
			})
			if err != nil {
				return fmt.Errorf("serve: failed to create server: %w", err)
			}

			return srv.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host address to bind to")
	cmd.Flags().IntVarP(&port, "port", "p", 8080, "TCP port to listen on")

	return cmd
}
