package server

import (
	"context"
	"io"
	"log/slog"
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
	// Logger is the structured logger used by the server and its handlers.
	// If nil, [logging.New] is used.
	Logger *slog.Logger
	// Pingers is the ordered list of dependency probes run by GET /api/ready.
	// If empty, /api/ready returns 200 with no checks (liveness-only mode).
	Pingers []Pinger
	// RateLimit is the sustained request rate allowed per IP on rate-limited
	// endpoints (requests/second). Defaults to 10 if zero.
	RateLimit float64
	// RateBurst is the maximum instantaneous burst per IP. Defaults to 20 if zero.
	RateBurst int
	// APIKey is the Bearer token required on all protected /api/* routes.
	// If empty, authentication is disabled (development mode).
	APIKey string
}

// querier is the interface handleChat calls to stream a response.
// *agent.TerraformAgent satisfies it; tests inject a fake.
type querier interface {
	// Query streams the agent response for userMessage to w.
	// Returns true if files were written to workspaceDir.
	Query(ctx context.Context, userMessage, workspaceDir string, w io.Writer) (bool, error)
}

// Server is the HTTP server that wraps the TerraformAgent.
type Server struct {
	// agent is the TF-AI agent that handles all queries.
	agent *agent.TerraformAgent
	// querier is the interface used by handleChat; set to agent in production,
	// overridden by a fake in tests.
	querier querier
	// cfg holds the resolved server configuration.
	cfg *Config
	// httpServer is the underlying net/http server.
	httpServer *http.Server
	// log is the structured logger for this server instance.
	log *slog.Logger
	// pingers is the ordered list of dependency probes for GET /api/ready.
	pingers []Pinger
	// stopRL stops the rate limiter's background eviction goroutine on shutdown.
	stopRL func()
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
	// Files is the recursive list of .tf and .tfvars files found under Dir,
	// returned as paths relative to Dir (e.g. "modules/vpc/main.tf").
	Files []string `json:"files"`
	// Dirs is kept for backward compatibility but is now always empty.
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

// fileResponse is the JSON response for GET /api/file.
type fileResponse struct {
	// Path is the absolute path of the file that was read.
	Path string `json:"path"`
	// Content is the raw file content.
	Content string `json:"content"`
}

// fileSaveRequest is the JSON body for PUT /api/file.
type fileSaveRequest struct {
	// WorkspaceDir is the declared workspace root. The path must resolve within it.
	WorkspaceDir string `json:"workspaceDir"`
	// Path is the absolute path of the file to write.
	Path string `json:"path"`
	// Content is the new file content to write.
	Content string `json:"content"`
}
