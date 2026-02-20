// Package server implements the HTTP server that exposes the TF-AI agent
// via a REST/SSE API and serves the embedded web UI.
// The server is started by the `tfai serve` CLI command.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/54b3r/tfai-go/internal/agent"
	"github.com/54b3r/tfai-go/internal/logging"
	"github.com/54b3r/tfai-go/internal/tracing"
)

// requestCounter is a monotonically increasing counter used to generate
// unique per-request session IDs for Langfuse traces.
var requestCounter atomic.Uint64

// New constructs a Server from the provided agent and config.
// If cfg.Logger is nil, [logging.New] is used.
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

	if cfg.Logger == nil {
		cfg.Logger = logging.New()
	}

	s := &Server{agent: tfAgent, cfg: cfg, log: cfg.Logger}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/workspace", s.handleWorkspace)
	mux.HandleFunc("POST /api/workspace/create", s.handleWorkspaceCreate)
	mux.HandleFunc("GET /api/file", s.handleFileRead)
	mux.HandleFunc("PUT /api/file", s.handleFileSave)
	// Resolve ui/static relative to the binary's working directory.
	// Using an absolute path avoids breakage when the binary is run from a
	// different working directory than the project root.
	uiDir, err := filepath.Abs("ui/static")
	if err != nil {
		return nil, fmt.Errorf("server: failed to resolve ui/static path: %w", err)
	}
	mux.Handle("/", http.FileServer(http.Dir(uiDir)))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      requestLogger(s.log, mux),
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
		s.log.Info("server listening", slog.String("addr", "http://"+s.httpServer.Addr))
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

// maxChatBodyBytes is the maximum allowed size for a /api/chat request body.
// Prevents unbounded memory allocation from oversized requests.
const maxChatBodyBytes = 1 << 20 // 1 MiB

// handleChat handles POST /api/chat requests. It streams the agent's response
// using Server-Sent Events (SSE) so the UI can render tokens as they arrive.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatBodyBytes)
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Validate workspaceDir is absolute if provided — same constraint as file API.
	if req.WorkspaceDir != "" && !filepath.IsAbs(filepath.Clean(req.WorkspaceDir)) {
		http.Error(w, "workspaceDir must be an absolute path", http.StatusBadRequest)
		return
	}

	// Set SSE headers so the client receives a streaming response.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Restrict CORS to the configured localhost origin only — this server is local-only.
	origin := r.Header.Get("Origin")
	allowedOrigin127 := fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port)
	allowedOriginLocal := fmt.Sprintf("http://localhost:%d", s.cfg.Port)
	if origin == allowedOrigin127 || origin == allowedOriginLocal || origin == "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stamp the request context with a unique session ID so each chat
	// request appears as a distinct named trace in Langfuse.
	sessionID := fmt.Sprintf("tfai-%d-%d", time.Now().UnixMilli(), requestCounter.Add(1))
	ctx := tracing.SetRequestTrace(r.Context(), sessionID)

	log := logging.FromContext(r.Context()).With(
		slog.String("session_id", sessionID),
		slog.String("workspace", req.WorkspaceDir),
	)
	log.Info("chat start", slog.String("message", req.Message))

	// sseWriter wraps the ResponseWriter to emit SSE-formatted data events.
	sw := &sseWriter{w: w, flusher: flusher}

	filesWritten, err := s.agent.Query(ctx, req.Message, req.WorkspaceDir, sw)
	if err != nil {
		log.Error("chat agent error", slog.Any("error", err))
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	if filesWritten {
		log.Info("chat files written")
		fmt.Fprintf(w, "event: files_written\ndata: true\n\n")
	}
	// Signal stream completion.
	fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}

// handleHealth handles GET /api/health for liveness checks.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		logging.FromContext(r.Context()).Error("health encode error", slog.Any("error", err))
	}
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
