package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GET /api/file — error paths
// ---------------------------------------------------------------------------

func TestHandleFileRead_MissingPath(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/file?workspaceDir=/tmp", nil)
	w := httptest.NewRecorder()

	s.handleFileRead(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileRead_MissingWorkspaceDir(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/file?path=/tmp/foo.tf", nil)
	w := httptest.NewRecorder()

	s.handleFileRead(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileRead_PathTraversal(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	// path escapes workspaceDir — must be rejected with 403
	req := httptest.NewRequest(http.MethodGet,
		"/api/file?path=/tmp/outside.tf&workspaceDir=/tmp/workspace", nil)
	w := httptest.NewRecorder()

	s.handleFileRead(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestHandleFileRead_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.tf")

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet,
		"/api/file?path="+missing+"&workspaceDir="+dir, nil)
	w := httptest.NewRecorder()

	s.handleFileRead(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/file — happy path
// ---------------------------------------------------------------------------

func TestHandleFileRead_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.tf")
	mustWriteFile(t, path, "# hello terraform")

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet,
		"/api/file?path="+path+"&workspaceDir="+dir, nil)
	w := httptest.NewRecorder()

	s.handleFileRead(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp fileResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp.Path != path {
		t.Errorf("Path: expected %q, got %q", path, resp.Path)
	}
	if resp.Content != "# hello terraform" {
		t.Errorf("Content: expected %q, got %q", "# hello terraform", resp.Content)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/file — error paths
// ---------------------------------------------------------------------------

func TestHandleFileSave_MissingPath(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/file",
		strings.NewReader(`{"workspaceDir":"/tmp","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleFileSave(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileSave_MissingWorkspaceDir(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/file",
		strings.NewReader(`{"path":"/tmp/foo.tf","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleFileSave(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileSave_PathTraversal(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/file",
		strings.NewReader(`{"path":"/etc/passwd","workspaceDir":"/tmp/workspace","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleFileSave(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestHandleFileSave_InvalidJSON(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/file",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleFileSave(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/file — happy path
// ---------------------------------------------------------------------------

func TestHandleFileSave_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.tf")
	body := `{"path":"` + path + `","workspaceDir":"` + dir + `","content":"# written by test"}`

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/file",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleFileSave(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	// Verify the file was actually written to disk with the correct content.
	got := mustReadFile(t, path)
	if got != "# written by test" {
		t.Errorf("file content: expected %q, got %q", "# written by test", got)
	}
}
