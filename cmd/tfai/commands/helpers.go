package commands

import (
	"github.com/cloudwego/eino/components/tool"

	tftools "github.com/54b3r/tfai-go/internal/tools"
)

// buildTools constructs the full list of Eino-compatible Terraform tools to
// register with the agent. If runner is nil, tools that require a live
// terraform binary are omitted gracefully.
//
// Note: terraform_generate is intentionally excluded. File generation is
// handled by parseAgentOutput + applyFiles in agent.Query(), which parses
// the JSON envelope from the LLM's text response directly.
func buildTools(runner tftools.Runner) []tool.BaseTool {
	var toolList []tool.BaseTool

	// plan and state tools require a live terraform binary.
	if runner != nil {
		toolList = append(toolList,
			tftools.NewPlanTool(runner),
			tftools.NewStateTool(runner),
		)
	}

	return toolList
}
