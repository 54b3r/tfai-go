package server

import (
	"net/http"
	"time"

	"github.com/54b3r/tfai-go/internal/agent"
)

// Config holds the HTTP server configuration.
type Config struct {
	// Host is the address to bind to (default: 127.0.0.1).
	Host string
	// Port is the TCP port to listen on (default: 8080).
	Port int
	// ReadTimeout is the maximum duration for reading the request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration for writing the response.
	WriteTimeout time.Duration
	// ShutdownTimeout is the maximum duration for a graceful shutdown.
	ShutdownTimeout time.Duration
}

// Server is the HTTP server that wraps the TerraformAgent.
type Server struct {
	// agent is the TF-AI agent that handles all queries.
	agent *agent.TerraformAgent
	// cfg holds the resolved server configuration.
	cfg *Config
	// httpServer is the underlying net/http server.
	httpServer *http.Server
}

// chatRequest is the JSON body for POST /api/chat.
type chatRequest struct {
	// Message is the user's natural language query.
	Message string `json:"message"`
	// WorkspaceDir is the directory to work in.
	WorkspaceDir string `json:"workspaceDir"`
}

// workspaceResponse is the JSON response for GET /api/workspace.
type workspaceResponse struct {
	// Dir is the cleaned absolute path that was inspected.
	Dir string `json:"dir"`
	// Files is the list of .tf and .tfvars filenames found in Dir.
	Files []string `json:"files"`
	// Dirs is the list of subdirectory names found in Dir (excluding hidden dirs).
	Dirs []string `json:"dirs"`
	// Initialized indicates a .terraform directory is present.
	Initialized bool `json:"initialized"`
	// HasState indicates a terraform.tfstate file is present.
	HasState bool `json:"hasState"`
	// HasLockfile indicates .terraform.lock.hcl is present.
	HasLockfile bool `json:"hasLockfile"`
}

// createWorkspaceRequest is the JSON body for POST /api/workspace/create.
type createWorkspaceRequest struct {
	// Dir is the absolute path of the directory to create.
	Dir string `json:"dir"`
	// Description is an optional hint for the LLM to pre-fill the chat.
	Description string `json:"description,omitempty"`
}

// createWorkspaceResponse is the JSON response for POST /api/workspace/create.
type createWorkspaceResponse struct {
	// Dir is the absolute path that was created.
	Dir string `json:"dir"`
	// Files is the list of scaffold files written.
	Files []string `json:"files"`
	// Prompt is a pre-filled chat prompt if Description was provided.
	Prompt string `json:"prompt,omitempty"`
}
