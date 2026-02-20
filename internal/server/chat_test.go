package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fake querier for chat handler tests
// ---------------------------------------------------------------------------

// fakeQuerier implements the querier interface for tests.
// It writes a fixed response to the writer and returns configurable values.
type fakeQuerier struct {
	// response is written verbatim to the writer on each Query call.
	response string
	// filesWritten is the value returned as the first return value.
	filesWritten bool
	// err is returned as the error value.
	err error
}

func (f *fakeQuerier) Query(_ context.Context, _, _ string, w io.Writer) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, _ = fmt.Fprint(w, f.response)
	return f.filesWritten, nil
}

// newChatTestServer builds a *Server wired with the given querier fake.
func newChatTestServer(q querier) *Server {
	return &Server{
		querier: q,
		cfg:     &Config{Port: 8080},
		log:     slog.Default(),
	}
}

// ---------------------------------------------------------------------------
// POST /api/chat — validation error paths (no agent needed)
// ---------------------------------------------------------------------------

func TestHandleChat_MissingMessage(t *testing.T) {
	t.Parallel()

	s := newChatTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"workspaceDir":"/tmp"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	t.Parallel()

	s := newChatTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_RelativeWorkspaceDir(t *testing.T) {
	t.Parallel()

	s := newChatTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"message":"hi","workspaceDir":"relative/path"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/chat — happy path (fake querier, SSE response)
// ---------------------------------------------------------------------------

// TestHandleChat_Success verifies that a valid request produces an SSE stream
// with a "done" event. httptest.ResponseRecorder implements http.Flusher so
// the handler's flusher check passes without a real connection.
func TestHandleChat_Success(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{response: "resource \"aws_s3_bucket\" \"b\" {}"}
	s := newChatTestServer(q)

	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"message":"generate an S3 bucket"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	body := w.Body.String()

	if !strings.Contains(body, "event: done") {
		t.Errorf("expected SSE done event in body, got: %s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("expected [DONE] sentinel in body, got: %s", body)
	}
}

// TestHandleChat_FilesWritten verifies that when the querier reports files
// were written, the SSE stream includes a "files_written" event.
func TestHandleChat_FilesWritten(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{response: "ok", filesWritten: true}
	s := newChatTestServer(q)

	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"message":"generate","workspaceDir":"/tmp"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: files_written") {
		t.Errorf("expected files_written event in body, got: %s", body)
	}
}

// TestHandleChat_AgentError verifies that when the querier returns an error,
// the SSE stream includes an "error" event and the response is still 200
// (SSE errors are delivered in-band, not via HTTP status).
func TestHandleChat_AgentError(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{err: fmt.Errorf("LLM unavailable")}
	s := newChatTestServer(q)

	req := httptest.NewRequest(http.MethodPost, "/api/chat",
		strings.NewReader(`{"message":"generate"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event in body, got: %s", body)
	}
	if !strings.Contains(body, "LLM unavailable") {
		t.Errorf("expected error message in body, got: %s", body)
	}
}
