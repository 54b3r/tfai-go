package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthMiddleware_Disabled verifies that when no API key is configured
// all requests pass through without an Authorization header.
func TestAuthMiddleware_Disabled(t *testing.T) {
	t.Parallel()

	h := authMiddleware("", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when auth disabled, got %d", w.Code)
	}
}

// TestAuthMiddleware_MissingHeader verifies that a request with no
// Authorization header receives 401 when auth is enabled.
func TestAuthMiddleware_MissingHeader(t *testing.T) {
	t.Parallel()

	h := authMiddleware("secret", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header on 401")
	}
}

// TestAuthMiddleware_WrongToken verifies that an incorrect Bearer token
// receives 401.
func TestAuthMiddleware_WrongToken(t *testing.T) {
	t.Parallel()

	h := authMiddleware("secret", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestAuthMiddleware_CorrectToken verifies that a valid Bearer token
// passes through to the downstream handler.
func TestAuthMiddleware_CorrectToken(t *testing.T) {
	t.Parallel()

	h := authMiddleware("secret", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestAuthMiddleware_CaseInsensitiveScheme verifies that "bearer" (lowercase)
// is accepted as well as "Bearer".
func TestAuthMiddleware_CaseInsensitiveScheme(t *testing.T) {
	t.Parallel()

	h := authMiddleware("secret", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/file", nil)
	req.Header.Set("Authorization", "bearer secret")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with lowercase bearer scheme, got %d", w.Code)
	}
}

// TestAuthMiddleware_MalformedHeader verifies that a non-Bearer Authorization
// header (e.g. Basic auth) is rejected with 401.
func TestAuthMiddleware_MalformedHeader(t *testing.T) {
	t.Parallel()

	h := authMiddleware("secret", okHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/file", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for Basic auth header, got %d", w.Code)
	}
}

// TestBearerToken verifies the bearerToken extraction helper.
func TestBearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		header string
		want   string
	}{
		{"Bearer mytoken", "mytoken"},
		{"bearer mytoken", "mytoken"},
		{"BEARER mytoken", "mytoken"},
		{"Bearer  spaced ", "spaced"},
		{"Basic dXNlcjpwYXNz", ""},
		{"", ""},
		{"Bearer", ""},
		{"token only", ""},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		got := bearerToken(req)
		if got != tc.want {
			t.Errorf("header=%q: expected %q, got %q", tc.header, tc.want, got)
		}
	}
}
