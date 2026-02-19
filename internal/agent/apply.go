package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func applyFiles(output *TerraformAgentOutput, workspaceDir string) error {

	// Loop over output.Files output by the agent and add them to filesystem
	for _, file := range output.Files {
		filePath := filepath.Join(workspaceDir, file.Path)
		// Safety check to ensure file is within workspace still
		if !strings.HasPrefix(filePath, workspaceDir) {
			return fmt.Errorf("agent::applyFiles: file path %s is outside workspace %s", filePath, workspaceDir)
		}
		// Create any subdirectories
		dir := filepath.Dir(filePath)
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("agent::applyFiles: failed to create directory %s: %w", dir, err)
			}
		}

		// Write file to disk
		if err := os.WriteFile(filePath, []byte(file.Content), 0644); err != nil {
			return fmt.Errorf("agent::applyFiles: failed to write file %s: %w", filePath, err)
		}

	}
	return nil
}
