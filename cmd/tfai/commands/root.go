// Package commands defines all Cobra CLI commands for the tfai binary.
package commands

import (
	"github.com/spf13/cobra"
)

// NewRootCmd constructs the root Cobra command that all subcommands attach to.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tfai",
		Short: "TF-AI — your expert Terraform engineer powered by LLMs",
		Long: `TF-AI is a local-first AI assistant for Terraform engineers and consultants.

It can generate Terraform code from natural language, diagnose plan/apply
failures, advise on state management, and answer infrastructure questions
across AWS, Azure, and GCP.

Model provider is selected via the MODEL_PROVIDER environment variable.
See 'tfai --help' for available commands.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		NewAskCmd(),
		NewGenerateCmd(),
		NewDiagnoseCmd(),
		NewServeCmd(),
		NewIngestCmd(),
	)

	return root
}
