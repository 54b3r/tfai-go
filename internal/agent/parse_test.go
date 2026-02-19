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

func TestParseAgentOutput(t *testing.T) {

	tests := []struct {
		name        string
		input       string
		wantFiles   int
		wantSummary string
		wantErr     bool
	}{
		{
			name:      "files only",
			input:     agentOutputFilesOnly,
			wantFiles: 2,
			wantErr:   false,
		}, {
			name:        "has summary",
			input:       agentOutputFull,
			wantFiles:   2,
			wantSummary: "This is a summary",
			wantErr:     false,
		}, {
			name:    "bad json",
			input:   agentOutputFail,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := parseAgentOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out.Files) != tt.wantFiles {
				t.Errorf("got %d files, want %d", len(out.Files), tt.wantFiles)
			}
			if out.Summary != tt.wantSummary {
				t.Errorf("got summary %q, want %q", out.Summary, tt.wantSummary)
			}
		})
	}
}
