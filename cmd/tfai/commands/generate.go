package commands

import (
	"fmt"
	"os"

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

			tfAgent, err := agent.New(ctx, &agent.Config{
				ChatModel: chatModel,
				Tools:     agentTools,
			})
			if err != nil {
				return fmt.Errorf("generate: failed to initialise agent: %w", err)
			}

			prompt := fmt.Sprintf(
				"Generate Terraform code for the following and write the files to directory %q:\n\n%s",
				outDir, args[0],
			)

			return tfAgent.Query(ctx, prompt, outDir, os.Stdout)
		},
	}

	cmd.Flags().StringVarP(&outDir, "out", "o", ".", "Output directory for generated .tf files")

	return cmd
}
