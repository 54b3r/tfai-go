// Command tfai is the entry point for the TF-AI Terraform expert agent.
// It provides a CLI interface (via Cobra) and an optional HTTP server with
// a web UI for interactive use.
package main

import (
	"fmt"
	"os"

	"github.com/54b3r/tfai-go/cmd/tfai/commands"
)

func main() {
	if err := commands.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
