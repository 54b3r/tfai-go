package commands

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/agent"
)

// NewDiagnoseCmd constructs the `tfai diagnose` command, which analyses a
// terraform plan output or apply error and provides a root-cause diagnosis
// with remediation steps.
func NewDiagnoseCmd() *cobra.Command {
	var planFile string
	var dir string

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose a terraform plan or apply failure",
		Long: `Diagnose a terraform plan output or apply error and receive a root-cause
analysis with step-by-step remediation guidance.

You can pipe plan output directly or provide a saved plan file.

Examples:
  terraform plan 2>&1 | tfai diagnose
  tfai diagnose --plan plan.txt
  tfai diagnose --dir ./infra --plan ./infra/plan.out`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Read plan content from file or stdin.
			var planContent string
			if planFile != "" {
				data, err := os.ReadFile(planFile)
				if err != nil {
					return fmt.Errorf("diagnose: failed to read plan file %q: %w", planFile, err)
				}
				planContent = string(data)
			} else {
				// Check if stdin has data (piped input).
				stat, err := os.Stdin.Stat()
				if err != nil {
					return fmt.Errorf("diagnose: failed to stat stdin: %w", err)
				}
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("diagnose: failed to read stdin: %w", err)
					}
					planContent = string(data)
				}
			}

			models, agentTools, _, _, err := initCommand(ctx)
			if err != nil {
				slog.Error("failed to initialize command", slog.String("command", cmd.Name()), slog.Any("error", err))
				return fmt.Errorf("diagnose: failed to initialize command: %w", err)
			}

			tfAgent, err := agent.New(ctx, &agent.Config{
				ChatModel: models.ChatModel,
				Tools:     agentTools,
			})
			if err != nil {
				return fmt.Errorf("diagnose: failed to initialise agent: %w", err)
			}

			var prompt string
			if planContent != "" {
				prompt = fmt.Sprintf(
					"Diagnose the following terraform output. Identify the root cause and provide step-by-step remediation:\n\n```\n%s\n```",
					planContent,
				)
			} else if dir != "" {
				prompt = fmt.Sprintf(
					"Run terraform plan in directory %q and diagnose any issues found.", dir,
				)
			} else {
				return fmt.Errorf("diagnose: provide --plan <file>, pipe plan output via stdin, or specify --dir <workspace>")
			}

			_, err = tfAgent.Query(ctx, prompt, "", os.Stdout)
			return err //nolint:wrapcheck // CLI entry point — error goes directly to cobra
		},
	}

	cmd.Flags().StringVarP(&planFile, "plan", "p", "", "Path to a saved terraform plan output file")
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "Terraform working directory to run plan against")

	return cmd
}
