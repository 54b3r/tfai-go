package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/agent"
	"github.com/54b3r/tfai-go/internal/provider"
	"github.com/54b3r/tfai-go/internal/tools"
)

// NewAskCmd constructs the `tfai ask` command, which sends a single natural
// language question to the agent and streams the response to stdout.
func NewAskCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask the Terraform expert a question",
		Long: `Ask the TF-AI agent a natural language question about Terraform.

The agent has access to your local Terraform workspace (set with --dir) and
can inspect plan output, state, and generated files.

Examples:
  tfai ask "how do I create an EKS cluster with private endpoints?"
  tfai ask --dir ./infra "why does my plan show resource replacement?"
  tfai ask "what is the best way to structure a multi-account AWS setup?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			chatModel, err := provider.NewFromEnv(ctx)
			if err != nil {
				return fmt.Errorf("ask: failed to initialise model provider: %w", err)
			}

			runner, err := tools.NewExecRunner()
			if err != nil {
				// terraform not on PATH is non-fatal for ask — warn and continue.
				fmt.Fprintf(os.Stderr, "warning: %v (plan/state tools unavailable)\n", err)
				runner = nil
			}

			agentTools := buildTools(runner)

			tfAgent, err := agent.New(ctx, &agent.Config{
				ChatModel: chatModel,
				Tools:     agentTools,
			})
			if err != nil {
				return fmt.Errorf("ask: failed to initialise agent: %w", err)
			}

			question := args[0]
			if dir != "" {
				question = fmt.Sprintf("[workspace: %s]\n\n%s", dir, question)
			}

			return tfAgent.Query(ctx, question, os.Stdout)
		},
	}

	cmd.Flags().StringVarP(&dir, "dir", "d", "", "Terraform working directory to use as context")

	return cmd
}
