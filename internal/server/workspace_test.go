// Package server contains tests for the workspace HTTP handlers and helpers.
//
// # Go Testing Primer
//
// Every test file ends in _test.go. The test runner (go test) automatically
// finds and runs any function whose name starts with Test.
//
// Key types:
//   - *testing.T  — the test handle. Call t.Errorf to record a failure and
//     continue, or t.Fatalf to record a failure and stop immediately.
//
// Running tests:
//
//	go test ./...                        # all tests in the module
//	go test -v ./internal/server/        # verbose output for this package
//	go test -run TestHandleWorkspace ... # run only tests matching a pattern
//	go test -race ./...                  # enable the data-race detector (always use in CI)
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest" // provides fake request/response — no real network needed
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------
//
// The easiest tests to write are for pure functions — functions that take
// inputs and return outputs with no side effects (no filesystem, no network).
// resolveAbsDir is a perfect example: given a string it either returns a
// cleaned path or an error. No setup or teardown required.

// TestResolveAbsDir uses the "table-driven" pattern — the most common Go test
// style. Instead of one function per case, define a slice of structs (the
// "table"), loop over them, and call t.Run to create a named sub-test per row.
//
// Benefits:
//   - Adding a new case is a single line in the table.
//   - Each sub-test has a descriptive name shown in failure output.
//   - Sub-tests can run in parallel independently.
func TestResolveAbsDir(t *testing.T) {
	// t.Parallel() tells the runner this test can run concurrently with other
	// parallel tests. Call it at the top of any test that doesn't share mutable
	// state — it speeds up the overall suite significantly.
	t.Parallel()

	tests := []struct {
		name    string // shown in test output as TestResolveAbsDir/<name>
		input   string // value passed to resolveAbsDir
		wantErr bool   // true if we expect a non-nil error back
	}{
		{name: "empty string", input: "", wantErr: true},
		{name: "relative path", input: "relative/path", wantErr: true},
		{name: "absolute path", input: "/tmp/some-dir", wantErr: false},
		// filepath.Clean strips the trailing slash, so this must still pass.
		{name: "absolute with trailing slash", input: "/tmp/some-dir/", wantErr: false},
	}

	for _, tc := range tests {
		tc := tc // capture loop variable — required in Go <1.22 to avoid data races
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolveAbsDir(tc.input)
			// (err != nil) is true when there IS an error. Comparing that bool
			// to wantErr catches both directions of failure in one expression.
			if (err != nil) != tc.wantErr {
				t.Errorf("resolveAbsDir(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// TestScaffoldFiles verifies that scaffoldFiles() always returns a non-empty
// list where every entry has both a name and content. This is a contract test —
// if someone accidentally removes a file or leaves a name blank, this catches
// it immediately rather than at runtime when a user creates a workspace.
func TestScaffoldFiles(t *testing.T) {
	t.Parallel()

	files := scaffoldFiles()

	// t.Fatal stops the test immediately. Use it when continuing would cause a
	// panic or make subsequent assertions meaningless.
	if len(files) == 0 {
		t.Fatal("scaffoldFiles() returned empty slice — at least one file is required")
	}
	for _, f := range files {
		if f.name == "" {
			// t.Error records the failure but continues — we want to check all entries.
			t.Error("scaffoldFiles() returned an entry with an empty name")
		}
		if f.content == "" {
			t.Errorf("scaffoldFiles() entry %q has empty content", f.name)
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------
//
// HTTP handlers have the signature: func(w http.ResponseWriter, r *http.Request)
//
// To test them without a real server, use net/http/httptest:
//   - httptest.NewRequest  — builds a fake *http.Request
//   - httptest.NewRecorder — a fake ResponseWriter that records status code,
//     headers, and body so we can assert on them after the handler returns.
//
// The pattern is always:
//  1. Build a fake request with httptest.NewRequest.
//  2. Create a recorder with httptest.NewRecorder.
//  3. Call the handler directly (no network, no goroutines, no port).
//  4. Assert on recorder.Code (status) and recorder.Body (response body).

// newTestServer builds a minimal *Server for handler tests.
// agent is nil because workspace handlers only touch the filesystem — they
// never call the LLM agent. If you test handleChat you'll need a real agent.
func newTestServer() *Server {
	return &Server{
		agent: nil,
		cfg:   &Config{},
	}
}

// ---------------------------------------------------------------------------
// GET /api/workspace — error path tests
// ---------------------------------------------------------------------------

// TestHandleWorkspace_MissingDir verifies that calling GET /api/workspace
// without a ?dir query parameter returns HTTP 400 Bad Request.
func TestHandleWorkspace_MissingDir(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	// httptest.NewRequest(method, url, body) — body is nil for GET requests.
	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	// NewRecorder() acts as the ResponseWriter. After the handler runs,
	// w.Code holds the HTTP status and w.Body holds the response body.
	w := httptest.NewRecorder()

	s.handleWorkspace(w, req) // call the handler directly — no server needed

	if w.Code != http.StatusBadRequest { // http.StatusBadRequest == 400
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

// TestHandleWorkspace_RelativePath verifies that a relative path in ?dir
// returns 400. Absolute paths are required to prevent ambiguity about what
// directory the path is relative to.
func TestHandleWorkspace_RelativePath(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	// Embed the query parameter directly in the URL string.
	req := httptest.NewRequest(http.MethodGet, "/api/workspace?dir=relative/path", nil)
	w := httptest.NewRecorder()

	s.handleWorkspace(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

// TestHandleWorkspace_NotFound verifies that an absolute path to a directory
// that doesn't exist on disk returns HTTP 404 Not Found.
func TestHandleWorkspace_NotFound(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	// The long random suffix makes accidental collision essentially impossible.
	req := httptest.NewRequest(http.MethodGet, "/api/workspace?dir=/tmp/tfai-does-not-exist-xyz-abc", nil)
	w := httptest.NewRecorder()

	s.handleWorkspace(w, req)

	if w.Code != http.StatusNotFound { // http.StatusNotFound == 404
		t.Errorf("expected 404 Not Found, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/workspace — happy path tests
// ---------------------------------------------------------------------------

// TestHandleWorkspace_EmptyDir verifies that a valid but empty directory
// returns 200 with empty files/dirs slices and all boolean flags false.
// This is a "happy path" test — everything is valid, we check the shape of
// the success response.
func TestHandleWorkspace_EmptyDir(t *testing.T) {
	t.Parallel()

	// t.TempDir() creates a temporary directory that is automatically deleted
	// when the test finishes. Always prefer this over os.MkdirTemp — cleanup
	// is guaranteed even if the test panics.
	dir := t.TempDir()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/workspace?dir="+dir, nil)
	w := httptest.NewRecorder()

	s.handleWorkspace(w, req)

	// t.Fatalf stops immediately — if we don't get 200 there's no point trying
	// to decode the body; it will be an error string, not a workspaceResponse.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d — body: %s", w.Code, w.Body.String())
	}

	// Decode the JSON response body into our response struct.
	// w.Body is a *bytes.Buffer so we pass it directly to json.NewDecoder.
	var resp workspaceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	// filepath.Clean normalises the path the same way the handler does,
	// making the comparison stable regardless of trailing slashes.
	if resp.Dir != filepath.Clean(dir) {
		t.Errorf("Dir: expected %q, got %q", filepath.Clean(dir), resp.Dir)
	}
	if len(resp.Files) != 0 {
		t.Errorf("Files: expected empty slice for empty directory, got %v", resp.Files)
	}
	if resp.Initialized {
		t.Error("Initialized: expected false — no .terraform/ directory present")
	}
	if resp.HasState {
		t.Error("HasState: expected false — no terraform.tfstate present")
	}
	if resp.HasLockfile {
		t.Error("HasLockfile: expected false — no .terraform.lock.hcl present")
	}
}

// TestHandleWorkspace_TFWorkspace verifies that a directory containing real
// Terraform artefacts is correctly detected. We create the files ourselves
// using test helpers so the test is fully self-contained and reproducible —
// it doesn't depend on any pre-existing directory on the developer's machine.
func TestHandleWorkspace_TFWorkspace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Set up a realistic Terraform workspace layout inside the temp dir.
	// mustWriteFile / mustMkdir are test helpers defined at the bottom of this
	// file. They call t.Fatal on error so tests fail fast with a clear message
	// instead of a cryptic nil-pointer panic later.
	mustWriteFile(t, filepath.Join(dir, "main.tf"), "# main")
	mustWriteFile(t, filepath.Join(dir, "terraform.tfstate"), "{}")
	mustWriteFile(t, filepath.Join(dir, ".terraform.lock.hcl"), "# lock")
	mustMkdir(t, filepath.Join(dir, ".terraform"))                      // presence signals `terraform init` was run
	mustMkdir(t, filepath.Join(dir, "modules"))                         // a visible (non-hidden) subdirectory
	mustWriteFile(t, filepath.Join(dir, "modules", "main.tf"), "# mod") // file inside subdir

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/workspace?dir="+dir, nil)
	w := httptest.NewRecorder()

	s.handleWorkspace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp workspaceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	// Assert every detection flag is set correctly.
	if !resp.Initialized {
		t.Error("Initialized: expected true — .terraform/ directory is present")
	}
	if !resp.HasState {
		t.Error("HasState: expected true — terraform.tfstate is present")
	}
	if !resp.HasLockfile {
		t.Error("HasLockfile: expected true — .terraform.lock.hcl is present")
	}

	// Files is now a recursive relative-path list. main.tf (root) and
	// modules/main.tf (subdir) should both appear. .terraform.lock.hcl is
	// hidden and terraform.tfstate is not a .tf/.tfvars file.
	wantFiles := map[string]bool{"main.tf": true, "modules/main.tf": true}
	if len(resp.Files) != len(wantFiles) {
		t.Errorf("Files: expected %v, got %v", wantFiles, resp.Files)
	}
	for _, f := range resp.Files {
		if !wantFiles[f] {
			t.Errorf("Files: unexpected entry %q", f)
		}
	}

	// Dirs is always empty in the new shape — files carry their own path.
	if len(resp.Dirs) != 0 {
		t.Errorf("Dirs: expected [], got %v", resp.Dirs)
	}
}

// ---------------------------------------------------------------------------
// POST /api/workspace/create — error path tests
// ---------------------------------------------------------------------------

// TestHandleWorkspaceCreate_MissingDir verifies that a POST body with no
// "dir" field returns 400. The handler validates dir before any filesystem ops.
func TestHandleWorkspaceCreate_MissingDir(t *testing.T) {
	t.Parallel()

	s := newTestServer()

	// For POST requests we pass a body. strings.NewReader converts a string
	// into an io.Reader, which is what http.Request.Body expects.
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/create",
		strings.NewReader(`{}`))
	// Always set Content-Type for JSON POST requests.
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleWorkspaceCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

// TestHandleWorkspaceCreate_RelativePath verifies that a relative "dir" value
// is rejected with 400 even when the JSON is otherwise valid.
func TestHandleWorkspaceCreate_RelativePath(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/create",
		strings.NewReader(`{"dir":"relative/path"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleWorkspaceCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

// TestHandleWorkspaceCreate_InvalidJSON verifies that a completely malformed
// request body returns 400 rather than panicking or returning 500.
// Never let bad input cause a 500 — that leaks implementation details.
func TestHandleWorkspaceCreate_InvalidJSON(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/create",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleWorkspaceCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/workspace/create — happy path test
// ---------------------------------------------------------------------------

// TestHandleWorkspaceCreate_Success is the most thorough test in this file.
// It verifies the full success path end-to-end:
//  1. The handler returns 200.
//  2. The JSON response contains the correct dir and a non-empty prompt.
//  3. Every scaffold file physically exists on disk after the call.
//  4. The response Files list matches what scaffoldFiles() declares.
func TestHandleWorkspaceCreate_Success(t *testing.T) {
	t.Parallel()

	// The directory must already exist — the handler no longer creates it.
	dir := filepath.Join(t.TempDir(), "new-workspace")
	mustMkdir(t, dir)
	body := `{"dir":"` + dir + `","description":"S3 bucket"}`

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/create",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleWorkspaceCreate(w, req)

	// Fatalf here — a non-200 body is an error string, not a createWorkspaceResponse.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp createWorkspaceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if resp.Dir != dir {
		t.Errorf("Dir: expected %q, got %q", dir, resp.Dir)
	}
	// A description was provided so the handler must have generated a prompt.
	if resp.Prompt == "" {
		t.Error("Prompt: expected non-empty string when description is provided")
	}

	// Cross-check the response against the actual filesystem — the files must
	// physically exist on disk, not just be listed in the JSON response.
	for _, f := range scaffoldFiles() {
		path := filepath.Join(dir, f.name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("scaffold file %q is listed in response but does not exist on disk", f.name)
		}
	}

	// The count of files in the response must match what scaffoldFiles() declares.
	if len(resp.Files) != len(scaffoldFiles()) {
		t.Errorf("Files count: expected %d, got %d", len(scaffoldFiles()), len(resp.Files))
	}
}

// TestHandleWorkspaceCreate_NonExistentDir verifies that the handler rejects
// a request for a directory that does not exist — we no longer create dirs.
func TestHandleWorkspaceCreate_NonExistentDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	body := `{"dir":"` + dir + `"}`

	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/create",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleWorkspaceCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------
//
// Test helpers are regular functions that accept *testing.T as their first
// argument. Two conventions make them work well:
//
//  1. t.Helper() — marks the function as a helper so that when it calls
//     t.Fatal, the failure is reported at the call site in the test, not
//     inside the helper. Without this, stack traces point to the wrong line.
//
//  2. Naming with "must" prefix — signals that the function will fail the
//     test immediately if the operation doesn't succeed. This makes test
//     setup code read clearly: "mustWriteFile means: write this file or fail".

// mustWriteFile writes content to path, failing the test immediately on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper() // report failures at the call site, not inside this function
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("mustWriteFile(%q): %v", path, err)
	}
}

// mustMkdir creates a directory (and any parents), failing the test immediately on error.
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mustMkdir(%q): %v", path, err)
	}
}
