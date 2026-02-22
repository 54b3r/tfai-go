package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/agent"
	"github.com/54b3r/tfai-go/internal/provider"
	"github.com/54b3r/tfai-go/internal/tools"
)

// NewGenerateCmd constructs the `tfai generate` command, which generates
// Terraform HCL files from a natural language description and writes them
// to the specified output directory.
func NewGenerateCmd() *cobra.Command {
	var outDir string

	cmd := &cobra.Command{
		Use:   "generate [description]",
		Short: "Generate Terraform code from a natural language description",
		Long: `Generate production-grade Terraform HCL files from a natural language description.

The agent will create appropriately structured .tf files (main.tf, variables.tf,
outputs.tf, versions.tf) in the specified output directory.

Examples:
  tfai generate "EKS cluster with IRSA, private endpoints, and managed node groups"
  tfai generate --out ./modules/aks "AKS cluster with Azure CNI and workload identity"
  tfai generate "GCS bucket with versioning, CMEK, and uniform bucket-level access"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			chatModel, err := provider.NewFromEnv(ctx)
			if err != nil {
				return fmt.Errorf("generate: failed to initialise model provider: %w", err)
			}

			runner, err := tools.NewExecRunner()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v (plan/state tools unavailable)\n", err)
				runner = nil
			}

			agentTools := buildTools(runner)

			retriever, closeRetriever := buildRetriever(ctx, slog.Default())
			defer closeRetriever()

			tfAgent, err := agent.New(ctx, &agent.Config{
				ChatModel: chatModel,
				Tools:     agentTools,
				Retriever: retriever,
			})
			if err != nil {
				return fmt.Errorf("generate: failed to initialise agent: %w", err)
			}

			absOutDir, err := filepath.Abs(outDir)
			if err != nil {
				return fmt.Errorf("generate: failed to resolve output directory: %w", err)
			}
			outDir = absOutDir

			prompt := fmt.Sprintf(
				"Generate production-grade Terraform code for the following and write the files to directory %q.\n\n"+
					"Requirements:\n"+
					"- Every resource and module block must have a comment above it explaining its purpose\n"+
					"- Every variable must have a description field and a sensible default where applicable\n"+
					"- Every output must have a description field\n"+
					"- Group related resources with section comment headers (e.g. # ── Networking ──)\n"+
					"- Use blank lines between blocks for readability\n"+
					"- Apply security best practices by default (encryption, least-privilege IAM, private endpoints)\n\n"+
					"Description: %s",
				outDir, args[0],
			)

			_, err = tfAgent.Query(ctx, prompt, outDir, os.Stdout) //nolint:wrapcheck // CLI entry point — error goes directly to cobra
			return err
		},
	}

	cmd.Flags().StringVarP(&outDir, "out", "o", ".", "Output directory for generated .tf files")

	return cmd
}
