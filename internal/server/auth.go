package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/54b3r/tfai-go/internal/logging"
)

// authMiddleware returns an HTTP middleware that enforces Bearer token
// authentication. If apiKey is empty the middleware is a no-op — auth is
// disabled and a warning is logged at server startup (not per-request).
//
// Protected routes must supply:
//
//	Authorization: Bearer <apiKey>
//
// Requests missing or presenting an incorrect token receive 401 Unauthorized
// with a WWW-Authenticate: Bearer challenge. The invalid token value is never
// logged — only its presence/absence is recorded.
func authMiddleware(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		// Auth disabled — pass all requests through unchanged.
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logging.FromContext(r.Context())

		token := bearerToken(r)
		if token == "" {
			log.Warn("auth: missing Authorization header",
				slog.String("path", r.URL.Path),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="tfai"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		if token != apiKey {
			log.Warn("auth: invalid token",
				slog.String("path", r.URL.Path),
				slog.Bool("token_present", true),
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="tfai" error="invalid_token"`)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header. Returns an empty string if the header is absent or malformed.
func bearerToken(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if hdr == "" {
		return ""
	}
	parts := strings.SplitN(hdr, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
