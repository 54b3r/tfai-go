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

// TestApplyFilesNoPathDoubling is a regression test for the bug where passing a
// relative --out directory caused the LLM to echo the path back into the JSON
// file paths, resulting in doubled nesting (e.g. test/foo/test/foo/main.tf).
// The fix is to resolve --out to an absolute path before injecting it into the
// prompt so the LLM receives an absolute path and returns relative file paths.
func TestApplyFilesNoPathDoubling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filePath  string // path returned by the LLM in the JSON envelope
		wantFile  string // expected file location relative to workspace root
		wantError bool
	}{
		{
			name:     "relative path — no doubling",
			filePath: "main.tf",
			wantFile: "main.tf",
		},
		{
			name:     "module subdir — no doubling",
			filePath: "modules/eks/main.tf",
			wantFile: "modules/eks/main.tf",
		},
		{
			name:     "llm echoes single dir segment — written under that subdir",
			filePath: "mydir/main.tf",
			wantFile: "mydir/main.tf",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			output := &TerraformAgentOutput{
				Summary: "regression: " + tc.name,
				Files:   []GeneratedFile{{Path: tc.filePath, Content: "# content"}},
			}

			err := applyFiles(output, dir)
			if tc.wantError {
				if err == nil {
					t.Errorf("applyFiles() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("applyFiles() unexpected error: %v", err)
			}

			want := filepath.Join(dir, tc.wantFile)
			if _, statErr := os.Stat(want); statErr != nil {
				t.Errorf("expected file at %s, got: %v", want, statErr)
			}

			// Verify no extra nesting: the file must not appear under a doubled path.
			doubled := filepath.Join(dir, filepath.Base(dir), tc.wantFile)
			if _, statErr := os.Stat(doubled); statErr == nil {
				t.Errorf("path doubling detected: file exists at doubled path %s", doubled)
			}
		})
	}
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
