package commands

import (
	"github.com/cloudwego/eino/components/tool"

	tftools "github.com/54b3r/tfai-go/internal/tools"
)

// buildTools constructs the full list of Eino-compatible Terraform tools to
// register with the agent. If runner is nil, tools that require a live
// terraform binary are omitted gracefully.
func buildTools(runner tftools.Runner) []tool.BaseTool {
	var toolList []tool.BaseTool

	// terraform_generate is always available — it only writes files to disk.
	toolList = append(toolList, tftools.NewGenerateTool())

	// plan and state tools require a live terraform binary.
	if runner != nil {
		toolList = append(toolList,
			tftools.NewPlanTool(runner),
			tftools.NewStateTool(runner),
		)
	}

	return toolList
}
