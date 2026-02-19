package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// ExecRunner implements Runner by executing the real terraform binary found
// on PATH. It is the default runner used in production.
type ExecRunner struct{}

// NewExecRunner returns a new ExecRunner. It verifies that the terraform
// binary is available on PATH at construction time.
func NewExecRunner() (*ExecRunner, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return nil, fmt.Errorf("tools: terraform binary not found on PATH — install terraform first")
	}
	return &ExecRunner{}, nil
}

// Run executes `terraform <subcommand> [args...]` in the workspace directory
// and returns the captured stdout, stderr, and exit code.
func (r *ExecRunner) Run(ctx context.Context, ws *WorkspaceContext, subcommand string, args ...string) (*RunResult, error) {
	cmdArgs := append([]string{subcommand}, args...)

	// Append any var-file flags.
	for _, vf := range ws.VarFiles {
		cmdArgs = append(cmdArgs, fmt.Sprintf("-var-file=%s", vf))
	}

	cmd := exec.CommandContext(ctx, "terraform", cmdArgs...)
	cmd.Dir = ws.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("tools: failed to run terraform %s: %w", subcommand, err)
		}
	}

	return &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}
