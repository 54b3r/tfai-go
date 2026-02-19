package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// PlanTool is an Eino tool that runs `terraform plan` in a given workspace
// directory and returns the plan output for the agent to analyse.
type PlanTool struct {
	// runner executes the terraform binary.
	runner Runner
}

// planInput is the JSON-serialisable input schema for PlanTool.
type planInput struct {
	// Dir is the absolute path to the Terraform working directory.
	Dir string `json:"dir"`

	// VarFiles is an optional list of .tfvars file paths.
	VarFiles []string `json:"var_files,omitempty"`

	// Destroy requests a destroy plan when true.
	Destroy bool `json:"destroy,omitempty"`
}

// NewPlanTool constructs a PlanTool using the provided Runner.
func NewPlanTool(runner Runner) *PlanTool {
	return &PlanTool{runner: runner}
}

// Name returns the tool name registered with the agent.
func (t *PlanTool) Name() string { return "terraform_plan" }

// Description returns the LLM-facing description of this tool.
func (t *PlanTool) Description() string {
	return "Runs `terraform plan` in the specified directory and returns the plan output. " +
		"Use this to preview infrastructure changes before applying them or to diagnose configuration issues."
}

// Info returns the Eino tool metadata including the JSON input schema.
func (t *PlanTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"dir": {
				Type:     schema.String,
				Desc:     "Absolute path to the Terraform working directory.",
				Required: true,
			},
			"var_files": {
				Type: schema.Array,
				Desc: "Optional list of .tfvars file paths to pass to terraform plan.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
			"destroy": {
				Type: schema.Boolean,
				Desc: "If true, generate a destroy plan instead of an apply plan.",
			},
		}),
	}, nil
}

// InvokableRun executes the tool given a JSON-encoded input string and returns
// the plan output as a string for the agent to consume.
func (t *PlanTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input planInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("terraform_plan: invalid input: %w", err)
	}
	if input.Dir == "" {
		return "", fmt.Errorf("terraform_plan: dir is required")
	}

	ws := &WorkspaceContext{
		Dir:      input.Dir,
		VarFiles: input.VarFiles,
	}

	args := []string{"-no-color"}
	if input.Destroy {
		args = append(args, "-destroy")
	}

	result, err := t.runner.Run(ctx, ws, "plan", args...)
	if err != nil {
		return "", fmt.Errorf("terraform_plan: execution failed: %w", err)
	}

	output := result.Stdout
	if result.Stderr != "" {
		output += "\n--- stderr ---\n" + result.Stderr
	}
	if result.ExitCode != 0 {
		return fmt.Sprintf("terraform plan exited with code %d:\n%s", result.ExitCode, output), nil
	}

	return output, nil
}
