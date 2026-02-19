package agent

import "testing"

// Testing constants for agent output parsing

const (
	agentOutputFilesOnly = `
{
  "files": [
    {
      "path": "main.tf",
      "content": "resource \"aws_instance\" \"web\" {\n  ami           = \"ami-12345678\"\n  instance_type = \"t2.micro\"\n}"
    },
	{
      "path": "variables.tf",
      "content": "variable \"instance_type\" {\n  type    = string\n  default = \"t2.micro\"\n}"
    }
  ]
}`
	agentOutputFull = `
{
  "summary": "This is a summary",
  "files": [
    {
      "path": "main.tf",
      "content": "resource \"aws_instance\" \"web\" {\n  ami           = \"ami-12345678\"\n  instance_type = \"t2.micro\"\n}"
    },
	{
      "path": "variables.tf",
      "content": "variable \"instance_type\" {\n  type    = string\n  default = \"t2.micro\"\n}"
    }
  ]
}`
	agentOutputFail = `This is not JSON`
)

func TestParseAgentOutputFiles(t *testing.T) {
	t.Parallel()

	output, err := parseAgentOutput(agentOutputFilesOnly)
	if err != nil {
		t.Errorf("parseAgentOutput() error = %v", err)
	}

	if len(output.Files) != 2 {
		t.Errorf("parseGeneratedFiles() length = %v, want 2", len(output.Files))
	}
}

func TestParseAgentOutputSummary(t *testing.T) {
	t.Parallel()

	output, err := parseAgentOutput(agentOutputFull)
	if err != nil {
		t.Errorf("parseAgentOutput() error = %v", err)
	}

	if output.Summary != "This is a summary" {
		t.Errorf("parseAgentOutput() summary = %v, want 'This is a summary'", output.Summary)
	}
}

func TestParseGeneratedFilesEmpty(t *testing.T) {
	t.Parallel()

	output, err := parseAgentOutput(agentOutputFail)
	if err == nil {
		t.Errorf("parseAgentOutput() expected error, got nil")
	}

	if output != nil {
		t.Errorf("parseAgentOutput() output = %v, want nil", output)
	}
}
