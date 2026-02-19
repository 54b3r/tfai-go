// Package tools defines the TerraformTool interface and all Terraform-specific
// tool implementations that the agent can invoke during a conversation.
// Each tool satisfies both this package's interface and Eino's tool.BaseTool
// interface so they can be registered directly with a ChatModelAgent.
package tools

import (
	"context"
)

// RunResult holds the output of a terraform CLI invocation.
type RunResult struct {
	// Stdout is the standard output captured from the terraform process.
	Stdout string

	// Stderr is the standard error captured from the terraform process.
	Stderr string

	// ExitCode is the process exit code (0 = success).
	ExitCode int
}

// TerraformTool is the interface that all Terraform-aware tools must satisfy.
// It extends the basic Eino tool contract with a Name accessor so the agent
// can log and route tool calls by name without type assertions.
type TerraformTool interface {
	// Name returns the unique tool name registered with the agent.
	Name() string

	// Description returns a human-readable description of what the tool does.
	// This text is sent to the LLM as part of the tool schema.
	Description() string
}

// WorkspaceContext carries the resolved path and optional configuration for
// the Terraform workspace the agent is currently operating on.
type WorkspaceContext struct {
	// Dir is the absolute path to the Terraform working directory.
	Dir string

	// VarFiles is an optional list of .tfvars files to pass to terraform commands.
	VarFiles []string

	// BackendConfig holds optional backend configuration overrides.
	BackendConfig map[string]string
}

// Runner is the interface for executing terraform CLI commands.
// Abstracting this allows tests to inject a fake runner without spawning
// real terraform processes.
type Runner interface {
	// Run executes the given terraform subcommand with args in the workspace dir.
	Run(ctx context.Context, ws *WorkspaceContext, subcommand string, args ...string) (*RunResult, error)
}
