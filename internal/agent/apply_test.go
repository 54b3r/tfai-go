package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	agentOutputModulePath = `
{
  "summary": "This is a summary",
  "files": [
    {
      "path": "module/foo/main.tf",
      "content": "resource \"aws_instance\" \"web\" {\n  ami           = \"ami-12345678\"\n  instance_type = \"t2.micro\"\n}"
    },
	{
      "path": "module/foo/variables.tf",
      "content": "variable \"instance_type\" {\n  type    = string\n  default = \"t2.micro\"\n}"
    }
	  ,
	{
      "path": "main.tf",
      "content": "variable \"instance_type\" {\n  type    = string\n  default = \"t2.micro\"\n}"
    }
  ]
}`
	agentOutputPathTraversal = `
{
  "summary": "This is a summary",
  "files": [
    {
      "path": "../../../etc/passwd",
      "content": "resource \"aws_instance\" \"web\" {\n  ami           = \"ami-12345678\"\n  instance_type = \"t2.micro\"\n}"
    }
  ]
}`
)

func returnAgentOutput(t *testing.T, agentOutputConst string) *TerraformAgentOutput {
	agentOutput, err := parseAgentOutput(agentOutputConst)
	if err != nil {
		t.Fatalf("Error parsing agent output: %v", err)
	}
	return agentOutput
}

func TestApplyFilesTwoFiles(t *testing.T) {
	t.Parallel()

	agentOutput := returnAgentOutput(t, agentOutputFilesOnly)

	// aoFiles := agentOutput.Files

	dir := t.TempDir() // Use TempdDir instead to ensure proper cleanup and keep things self contained
	err := applyFiles(agentOutput, dir)
	if err != nil {
		t.Errorf("applyFiles() error = %v", err)
	}

	// For each file that was applied, check if it actually exists on the filesystem
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		_, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Errorf("Failed to read file %s: %v", entry.Name(), err)
		}
	}

}

func TestApplyFilesModulePath(t *testing.T) {
	t.Parallel()

	// Not sure if this type of testing is acceptable, but it shows the true behavior with an actual
	// agent output that has been parsed by the code
	agentOutput := returnAgentOutput(t, agentOutputModulePath)
	dir := t.TempDir() // Use TempdDir instead to ensure proper cleanup and keep things self contained
	err := applyFiles(agentOutput, dir)
	if err != nil {
		t.Errorf("applyFiles() error = %v", err)
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("ReadDir() error = %v", err)
	}

	if len(dirEntries) == 0 {
		t.Error("Expected directory entries, got none")
	}

	t.Logf("Directory entries: %v", dirEntries)
}

func TestApplyFilesPathTraversal(t *testing.T) {
	t.Parallel()

	agentOutput := returnAgentOutput(t, agentOutputPathTraversal)

	dir := t.TempDir() // Use TempdDir instead to ensure proper cleanup and keep things self contained
	err := applyFiles(agentOutput, dir)
	contains := "agent::applyFiles: file path "
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Errorf("applyFiles() error = %v", err)
	}
	for _, file := range agentOutput.Files {
		path := file.Path
		containsTraversal := strings.Contains(path, "..")

		if !containsTraversal {
			t.Errorf("expected Path traversal to be detected: Path: %s, containsTraversal: %v", path, containsTraversal)
		}
	}
}
