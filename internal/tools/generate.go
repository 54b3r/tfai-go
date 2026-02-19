package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// GenerateTool is an Eino tool that writes LLM-generated Terraform HCL files
// to a target directory on the local filesystem. The agent produces the HCL
// content and this tool persists it, keeping file I/O out of the LLM context.
type GenerateTool struct{}

// generateInput is the JSON-serialisable input schema for GenerateTool.
type generateInput struct {
	// Dir is the absolute path to the directory where files will be written.
	Dir string `json:"dir"`

	// Files is a map of filename → HCL content to write.
	Files map[string]string `json:"files"`
}

// NewGenerateTool constructs a GenerateTool.
func NewGenerateTool() *GenerateTool {
	return &GenerateTool{}
}

// Name returns the tool name registered with the agent.
func (t *GenerateTool) Name() string { return "terraform_generate" }

// Description returns the LLM-facing description of this tool.
func (t *GenerateTool) Description() string {
	return "Writes Terraform HCL files to a specified directory on the local filesystem. " +
		"Provide a map of filename to HCL content. " +
		"Use this after generating Terraform code to persist it for the user."
}

// Info returns the Eino tool metadata including the JSON input schema.
func (t *GenerateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"dir": {
				Type:     schema.String,
				Desc:     "Absolute path to the directory where .tf files will be written. Created if it does not exist.",
				Required: true,
			},
			"files": {
				Type:     schema.Object,
				Desc:     "Map of filename (e.g. 'main.tf') to HCL content string.",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun writes the provided HCL files to disk and returns a summary.
func (t *GenerateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input generateInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("terraform_generate: invalid input: %w", err)
	}
	if input.Dir == "" {
		return "", fmt.Errorf("terraform_generate: dir is required")
	}
	if len(input.Files) == 0 {
		return "", fmt.Errorf("terraform_generate: files map must not be empty")
	}

	if err := os.MkdirAll(input.Dir, 0o755); err != nil {
		return "", fmt.Errorf("terraform_generate: failed to create directory %q: %w", input.Dir, err)
	}

	written := make([]string, 0, len(input.Files))
	for name, content := range input.Files {
		path := filepath.Join(input.Dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("terraform_generate: failed to write %q: %w", path, err)
		}
		written = append(written, path)
	}

	return fmt.Sprintf("Successfully wrote %d file(s) to %s:\n%v", len(written), input.Dir, written), nil
}
