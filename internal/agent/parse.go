package agent

import (
	"encoding/json"
	"fmt"
)

// parseAgentOutput takes an input string of generated text from the terrafrom agent tools
// and extracts the file path, along with the raw HCL for each given file generated for the
// returned tf solution
func parseAgentOutput(output string) (*TerraformAgentOutput, error) {
	agentOutput := &TerraformAgentOutput{}

	err := json.Unmarshal([]byte(output), agentOutput)
	if err != nil {
		return nil, fmt.Errorf("agent::parseAgentOutput: failed to unmarshal agent output: %w", err)
	}

	return agentOutput, nil
}
