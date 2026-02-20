package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Fake Pinger for readiness tests
// ---------------------------------------------------------------------------

// fakePinger is a test double for the Pinger interface.
type fakePinger struct {
	// name is returned by Name().
	name string
	// err is returned by Ping(); nil means healthy.
	err error
}

func (f *fakePinger) Name() string                 { return f.name }
func (f *fakePinger) Ping(_ context.Context) error { return f.err }

// newReadyTestServer builds a *Server with the given pingers wired in.
func newReadyTestServer(pingers ...Pinger) *Server {
	s := newTestServer()
	s.pingers = pingers
	return s
}

// ---------------------------------------------------------------------------
// GET /api/health — liveness
// ---------------------------------------------------------------------------

// TestHandleHealth_OK verifies that GET /api/health returns 200 with a JSON
// body containing {"status":"ok"}.
func TestHandleHealth_OK(t *testing.T) {
	t.Parallel()

	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d — body: %s", w.Code, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: expected application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status: expected %q, got %q", "ok", body["status"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/ready — readiness
// ---------------------------------------------------------------------------

// TestHandleReady_NoPingers verifies that /api/ready returns 200 with
// ready:true and an empty checks array when no pingers are registered.
func TestHandleReady_NoPingers(t *testing.T) {
	t.Parallel()

	s := newReadyTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp readyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ready {
		t.Errorf("expected ready:true with no pingers")
	}
	if len(resp.Checks) != 0 {
		t.Errorf("expected 0 checks, got %d", len(resp.Checks))
	}
}

// TestHandleReady_AllHealthy verifies that /api/ready returns 200 with
// ready:true when all pingers succeed.
func TestHandleReady_AllHealthy(t *testing.T) {
	t.Parallel()

	s := newReadyTestServer(
		&fakePinger{name: "llm", err: nil},
		&fakePinger{name: "qdrant", err: nil},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp readyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Ready {
		t.Errorf("expected ready:true")
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(resp.Checks))
	}
	for _, c := range resp.Checks {
		if !c.OK {
			t.Errorf("check %q: expected ok:true", c.Name)
		}
		if c.Error != "" {
			t.Errorf("check %q: expected no error, got %q", c.Name, c.Error)
		}
	}
}

// TestHandleReady_OneFailing verifies that /api/ready returns 503 with
// ready:false when one pinger fails, and the failing check has ok:false
// with a non-empty error field.
func TestHandleReady_OneFailing(t *testing.T) {
	t.Parallel()

	s := newReadyTestServer(
		&fakePinger{name: "llm", err: nil},
		&fakePinger{name: "qdrant", err: errors.New("connection refused")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp readyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Ready {
		t.Errorf("expected ready:false")
	}

	var qdrantCheck *readyCheck
	for i := range resp.Checks {
		if resp.Checks[i].Name == "qdrant" {
			qdrantCheck = &resp.Checks[i]
		}
	}
	if qdrantCheck == nil {
		t.Fatal("qdrant check missing from response")
	}
	if qdrantCheck.OK {
		t.Errorf("qdrant check: expected ok:false")
	}
	if qdrantCheck.Error == "" {
		t.Errorf("qdrant check: expected non-empty error")
	}
}

// TestHandleReady_AllFailing verifies that /api/ready returns 503 with
// ready:false and all checks showing ok:false when every pinger fails.
func TestHandleReady_AllFailing(t *testing.T) {
	t.Parallel()

	s := newReadyTestServer(
		&fakePinger{name: "llm", err: errors.New("timeout")},
		&fakePinger{name: "qdrant", err: errors.New("connection refused")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp readyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Ready {
		t.Errorf("expected ready:false")
	}
	for _, c := range resp.Checks {
		if c.OK {
			t.Errorf("check %q: expected ok:false", c.Name)
		}
	}
}

// TestHandleReady_ContentType verifies the response always has Content-Type
// application/json regardless of probe outcome.
func TestHandleReady_ContentType(t *testing.T) {
	t.Parallel()

	s := newReadyTestServer(&fakePinger{name: "llm", err: errors.New("down")})
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	w := httptest.NewRecorder()

	s.handleReady(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: expected application/json, got %q", ct)
	}
}
