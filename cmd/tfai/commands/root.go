// Package commands defines all Cobra CLI commands for the tfai binary.
package commands

import (
	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/audit"
	"github.com/54b3r/tfai-go/internal/config"
	"github.com/54b3r/tfai-go/internal/logging"
)

// configPath holds the --config flag value for YAML config file override.
var configPath string

// loadedConfigPath stores the resolved config file path for audit logging.
var loadedConfigPath string

// NewRootCmd constructs the root Cobra command that all subcommands attach to.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tfai",
		Short: "TF-AI — your expert Terraform engineer powered by LLMs",
		Long: `TF-AI is a local-first AI assistant for Terraform engineers and consultants.

It can generate Terraform code from natural language, diagnose plan/apply
failures, advise on state management, and answer infrastructure questions
across AWS, Azure, and GCP.

Model provider is selected via the MODEL_PROVIDER environment variable
or a YAML config file (~/.tfai/config.yaml).
See 'tfai --help' for available commands.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			log := logging.New()

			// Load YAML config (env vars always override YAML values).
			path, err := config.Load(configPath, log)
			if err != nil {
				return err
			}
			loadedConfigPath = path

			// Emit structured audit log for every command invocation.
			audit.LogCommandStart(log, cmd.Name(), loadedConfigPath)

			return nil
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "Path to YAML config file (default: ~/.tfai/config.yaml)")

	root.AddCommand(
		NewAskCmd(),
		NewGenerateCmd(),
		NewDiagnoseCmd(),
		NewServeCmd(),
		NewIngestCmd(),
		NewVersionCmd(),
	)

	return root
}
