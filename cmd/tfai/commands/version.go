package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/version"
)

// NewVersionCmd constructs the `tfai version` subcommand.
// It prints the binary version, git commit, and build date injected at
// build time via -ldflags. Falls back to "dev"/"unknown" for local builds.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the tfai version, git commit, and build date",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tfai %s (commit: %s, built: %s)\n",
				version.Version, version.Commit, version.BuildDate)
		},
	}
}
