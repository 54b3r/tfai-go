package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/54b3r/tfai-go/internal/logging"
)

// requestLogger is an [http.Handler] middleware that:
//  1. Generates a unique request_id for every inbound request.
//  2. Injects a child [*slog.Logger] carrying that ID into the request context.
//  3. Logs method, path, status code, and latency on completion.
func requestLogger(base *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := newRequestID()

		log := base.With(
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		)

		ctx := logging.WithLogger(r.Context(), log)
		r = r.WithContext(ctx)

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		start := time.Now()
		next.ServeHTTP(rw, r)
		elapsed := time.Since(start)

		log.Info("request",
			slog.Int("status", rw.status),
			slog.Duration("duration", elapsed),
		)
	})
}

// responseWriter wraps [http.ResponseWriter] to capture the status code
// written by the handler so the middleware can log it.
type responseWriter struct {
	http.ResponseWriter
	// status is the HTTP status code sent to the client.
	status int
}

// WriteHeader captures the status code before delegating to the underlying writer.
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// newRequestID returns a 16-byte cryptographically random hex string.
// Falls back to a zero-filled ID on the (impossible in practice) error path.
func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}
