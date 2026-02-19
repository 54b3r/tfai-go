// Package server implements the HTTP server that exposes the TF-AI agent
// via a REST/SSE API and serves the embedded web UI.
// The server is started by the `tfai serve` CLI command.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log"

	"github.com/54b3r/tfai-go/internal/agent"
)

// New constructs a Server from the provided agent and config.
func New(tfAgent *agent.TerraformAgent, cfg *Config) (*Server, error) {
	if tfAgent == nil {
		return nil, fmt.Errorf("server: agent must not be nil")
	}
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		// WriteTimeout must be long enough for streaming responses.
		cfg.WriteTimeout = 5 * time.Minute
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	s := &Server{agent: tfAgent, cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/workspace", s.handleWorkspace)
	mux.HandleFunc("POST /api/workspace/create", s.handleWorkspaceCreate)
	mux.Handle("/", http.FileServer(http.Dir("ui/static")))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s, nil
}

// Start begins listening and serving HTTP requests. It blocks until the
// context is cancelled, then performs a graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		fmt.Printf("tfai server listening on http://%s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server: listen error: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown failed: %w", err)
		}
		return nil
	}
}

// handleChat handles POST /api/chat requests. It streams the agent's response
// using Server-Sent Events (SSE) so the UI can render tokens as they arrive.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Set SSE headers so the client receives a streaming response.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// sseWriter wraps the ResponseWriter to emit SSE-formatted data events.
	sw := &sseWriter{w: w, flusher: flusher}

	if err := s.agent.Query(r.Context(), req.Message, sw); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Signal stream completion.
	fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

// handleHealth handles GET /api/health for liveness checks.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// sseWriter wraps an http.ResponseWriter to emit Server-Sent Event data frames.
type sseWriter struct {
	// w is the underlying response writer.
	w http.ResponseWriter

	// flusher flushes buffered data to the client after each write.
	flusher http.Flusher
}

// Write formats p as one or more SSE data lines and flushes to the client.
// Each newline in p is prefixed with "data: " so multi-line chunks never
// break the SSE frame boundary.
func (s *sseWriter) Write(p []byte) (n int, err error) {
	chunk := strings.TrimRight(string(bytes.Clone(p)), "\n")
	lines := strings.Split(chunk, "\n")
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString("data: ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
	if _, err = fmt.Fprint(s.w, buf.String()); err != nil {
		return 0, err
	}
	s.flusher.Flush()
	return len(p), nil
}

func decodeCreateWorkspaceRequest(r *http.Request) (createWorkspaceRequest, error) {
	var req createWorkspaceRequest
	body := r.Body
	defer body.Close()
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return createWorkspaceRequest{}, err
	}

	if req.Dir == "" {
		return createWorkspaceRequest{}, fmt.Errorf("dir is required")
	}

	return req, nil
}

// handleWorkspaceCreate handles POST /api/workspace/create
func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeCreateWorkspaceRequest(r)
	if err != nil {
		log.Printf("server: workspace create decode error: %v", err)
		http.Error(w, `{"error":"invalid request body: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	resp := createWorkspaceResponse{Dir: req.Dir}

	if req.Description != "" {
		resp.Prompt = "Create a Terraform workspace for: " + req.Description
	}

	returnFilePath := func(fileName string) string {
		return filepath.Join(req.Dir, fileName)
	}

	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		log.Printf("server: workspace create mkdir error: %v", err)
		http.Error(w, `{"error":"failed to create directory: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	scaffoldFiles := []struct {
		name    string
		content string
	}{
		{"main.tf", "# Add your resources here\n"},
		{"variables.tf", "# Define input variables here\n"},
		{"outputs.tf", "# Define outputs here\n"},
		{"versions.tf", "terraform {\n  required_version = \">= 1.5\"\n}\n"},
	}
	for _, f := range scaffoldFiles {
		if err := os.WriteFile(returnFilePath(f.name), []byte(f.content), 0o644); err != nil {
			log.Printf("server: workspace create write %s error: %v", f.name, err)
			http.Error(w, `{"error":"failed to create `+f.name+`: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		resp.Files = append(resp.Files, f.name)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("server: workspace create encode error: %v", err)
	}
}

// handleWorkspace handles GET /api/workspace?dir=<path>.
func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("dir")
	if raw == "" {
		http.Error(w, `{"error":"dir query parameter is required"}`, http.StatusBadRequest)
		return
	}

	dir := filepath.Clean(raw)
	if !filepath.IsAbs(dir) {
		http.Error(w, `{"error":"dir must be an absolute path"}`, http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, `{"error":"directory not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to read directory"}`, http.StatusInternalServerError)
		return
	}

	resp := workspaceResponse{
		Dir:   dir,
		Files: []string{},
		Dirs:  []string{},
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if name == ".terraform" {
				resp.Initialized = true
			}
			// Skip all hidden directories from the Dirs list
			if !strings.HasPrefix(name, ".") {
				resp.Dirs = append(resp.Dirs, name)
			}
			continue
		}
		switch name {
		case "terraform.tfstate":
			resp.HasState = true
		case ".terraform.lock.hcl":
			resp.HasLockfile = true
		}
		ext := filepath.Ext(name)
		if ext == ".tf" || ext == ".tfvars" {
			resp.Files = append(resp.Files, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("server: workspace encode error: %v\n", err)
	}
}
