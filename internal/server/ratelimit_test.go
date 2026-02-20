package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is a trivial handler used to verify that allowed requests reach
// the downstream handler.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// TestRateLimit_AllowsUnderLimit verifies that requests within the burst
// capacity are passed through to the downstream handler.
func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	t.Parallel()

	rl, stop := newRateLimiter(100, 5, slog.Default())
	defer stop()

	h := rl.middleware(okHandler)

	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/api/file", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, w.Code)
		}
	}
}

// TestRateLimit_BlocksOverLimit verifies that requests exceeding the burst
// capacity receive 429 Too Many Requests.
func TestRateLimit_BlocksOverLimit(t *testing.T) {
	t.Parallel()

	// burst=2, rps=0.001 — third request must be rejected immediately.
	rl, stop := newRateLimiter(0.001, 2, slog.Default())
	defer stop()

	h := rl.middleware(okHandler)

	got429 := false
	for i := range 10 {
		req := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			_ = i
			break
		}
	}
	if !got429 {
		t.Error("expected at least one 429 response, got none")
	}
}

// TestRateLimit_RetryAfterHeader verifies that 429 responses include a
// Retry-After header.
func TestRateLimit_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	rl, stop := newRateLimiter(0.001, 1, slog.Default())
	defer stop()

	h := rl.middleware(okHandler)

	// First request consumes the single burst token.
	req := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	h.ServeHTTP(httptest.NewRecorder(), req)

	// Second request must be rejected with Retry-After.
	req2 := httptest.NewRequest(http.MethodPost, "/api/chat", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

// TestRateLimit_PerIPIsolation verifies that two different IPs have
// independent token buckets — exhausting one does not affect the other.
func TestRateLimit_PerIPIsolation(t *testing.T) {
	t.Parallel()

	rl, stop := newRateLimiter(0.001, 1, slog.Default())
	defer stop()

	h := rl.middleware(okHandler)

	// Exhaust IP A.
	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/api/file", nil)
		req.RemoteAddr = "192.168.1.1:1111"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	// IP B should still be allowed.
	req := httptest.NewRequest(http.MethodGet, "/api/file", nil)
	req.RemoteAddr = "192.168.1.2:2222"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP B: expected 200, got %d — should be independent of IP A", w.Code)
	}
}

// TestClientIP verifies that clientIP strips the port from RemoteAddr.
func TestClientIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		remoteAddr string
		wantIP     string
	}{
		{"127.0.0.1:54321", "127.0.0.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"::1:8080", "::1"},
		{"noport", "noport"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remoteAddr
		got := clientIP(req)
		if got != tc.wantIP {
			t.Errorf("remoteAddr=%q: expected %q, got %q", tc.remoteAddr, tc.wantIP, got)
		}
	}
}
