package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// StateTool is an Eino tool that reads and analyses the Terraform state for a
// given workspace. It supports listing resources, showing individual resource
// state, and pulling the raw state JSON for deeper inspection.
type StateTool struct {
	// runner executes the terraform binary.
	runner Runner
}

// stateInput is the JSON-serialisable input schema for StateTool.
type stateInput struct {
	// Dir is the absolute path to the Terraform working directory.
	Dir string `json:"dir"`

	// Subcommand is the state sub-operation: "list", "show", or "pull".
	Subcommand string `json:"subcommand"`

	// Resource is the resource address for "show" (e.g. "aws_eks_cluster.main").
	Resource string `json:"resource,omitempty"`
}

// NewStateTool constructs a StateTool using the provided Runner.
func NewStateTool(runner Runner) *StateTool {
	return &StateTool{runner: runner}
}

// Name returns the tool name registered with the agent.
func (t *StateTool) Name() string { return "terraform_state" }

// Description returns the LLM-facing description of this tool.
func (t *StateTool) Description() string {
	return "Inspects the Terraform state for a workspace. " +
		"Supports subcommands: 'list' (list all managed resources), " +
		"'show' (show state for a specific resource address), " +
		"'pull' (return the raw state JSON). " +
		"Use this to diagnose state drift, missing resources, or corrupted state."
}

// Info returns the Eino tool metadata including the JSON input schema.
func (t *StateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"dir": {
				Type:     schema.String,
				Desc:     "Absolute path to the Terraform working directory.",
				Required: true,
			},
			"subcommand": {
				Type:     schema.String,
				Desc:     "State sub-operation: 'list', 'show', or 'pull'.",
				Required: true,
			},
			"resource": {
				Type: schema.String,
				Desc: "Resource address for 'show' subcommand (e.g. 'aws_eks_cluster.main').",
			},
		}),
	}, nil
}

// InvokableRun executes the tool given a JSON-encoded input string.
func (t *StateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input stateInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("terraform_state: invalid input: %w", err)
	}
	if input.Dir == "" {
		return "", fmt.Errorf("terraform_state: dir is required")
	}

	ws := &WorkspaceContext{Dir: input.Dir}

	var args []string
	switch input.Subcommand {
	case "list":
		args = []string{"list"}
	case "show":
		if input.Resource == "" {
			return "", fmt.Errorf("terraform_state: resource is required for 'show' subcommand")
		}
		args = []string{"show", "-no-color", input.Resource}
	case "pull":
		args = []string{"pull"}
	default:
		return "", fmt.Errorf("terraform_state: unknown subcommand %q — valid values: list, show, pull", input.Subcommand)
	}

	result, err := t.runner.Run(ctx, ws, "state", args...)
	if err != nil {
		return "", fmt.Errorf("terraform_state: execution failed: %w", err)
	}

	output := result.Stdout
	if result.Stderr != "" {
		output += "\n--- stderr ---\n" + result.Stderr
	}
	if result.ExitCode != 0 {
		return fmt.Sprintf("terraform state %s exited with code %d:\n%s", input.Subcommand, result.ExitCode, output), nil
	}

	return output, nil
}
