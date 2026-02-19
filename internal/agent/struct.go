package agent

// GeneratedFile Struct is used to define the json schema for a file being used
// to store generated terraform code from the agent execution output
type GeneratedFile struct {
	// Path is the value of the path to the file being created + "filename"
	Path string `json:"path"`
	// Content is where the raw HCL code is stored from the generated agent output that will be
	// inserted into the respective .tf file
	Content string `json:"content"`
}

type TerraformAgentOutput struct {
	// Files holds a slice of the generated files
	Files []GeneratedFile `json:"files"`
	// Summary holds the summary of the generated files
	Summary string `json:"summary"`
}
