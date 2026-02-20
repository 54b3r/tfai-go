package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/54b3r/tfai-go/internal/logging"
)

// probeTimeout is the maximum time allowed for each individual dependency
// probe during a readiness check. Kept short so /api/ready responds quickly
// even when a dependency is slow rather than unreachable.
const probeTimeout = 5 * time.Second

// Pinger is the interface implemented by any dependency that can report its
// own reachability. Each implementation must return nil when the dependency
// is healthy and a descriptive error otherwise.
// Implementations must be safe to call from multiple goroutines.
type Pinger interface {
	// Ping checks whether the dependency is reachable within the given context.
	// Returns nil on success, a descriptive error on failure.
	Ping(ctx context.Context) error

	// Name returns a short human-readable label used in readiness responses
	// (e.g. "ollama", "qdrant").
	Name() string
}

// MultiPinger aggregates one or more Pinger implementations and reports
// the combined readiness of all dependencies.
type MultiPinger struct {
	// pingers is the ordered list of dependency probes to run.
	pingers []Pinger
}

// NewMultiPinger constructs a MultiPinger from the provided list of Pingers.
func NewMultiPinger(pingers ...Pinger) *MultiPinger {
	return &MultiPinger{pingers: pingers}
}

// Ping runs all registered probes sequentially and returns the first error
// encountered, or nil if all probes succeed.
func (m *MultiPinger) Ping(ctx context.Context) error {
	for _, p := range m.pingers {
		if err := p.Ping(ctx); err != nil {
			return fmt.Errorf("%s: %w", p.Name(), err)
		}
	}
	return nil
}

// Name returns a combined label for logging purposes.
func (m *MultiPinger) Name() string { return "multi" }

// readyCheck holds the per-dependency result of a readiness probe.
type readyCheck struct {
	// Name is the dependency label (e.g. "ollama", "qdrant").
	Name string `json:"name"`
	// OK is true when the dependency responded successfully.
	OK bool `json:"ok"`
	// Error contains the failure reason when OK is false. Empty on success.
	Error string `json:"error,omitempty"`
}

// readyResponse is the JSON body returned by GET /api/ready.
type readyResponse struct {
	// Ready is true only when every dependency probe succeeded.
	Ready bool `json:"ready"`
	// Checks contains the per-dependency probe results.
	Checks []readyCheck `json:"checks"`
}

// handleReady handles GET /api/ready for readiness checks.
// It probes each registered Pinger with a short timeout and returns 200 when
// all dependencies are reachable, or 503 when any probe fails.
// Unlike /api/health (liveness), this endpoint reflects actual dependency state.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	resp := readyResponse{Ready: true}
	allOK := true

	for _, p := range s.pingers {
		probeCtx, cancel := context.WithTimeout(r.Context(), probeTimeout)
		err := p.Ping(probeCtx)
		cancel()

		check := readyCheck{Name: p.Name(), OK: err == nil}
		if err != nil {
			check.Error = err.Error()
			allOK = false
			log.Warn("readiness probe failed",
				slog.String("dependency", p.Name()),
				slog.Any("error", err),
			)
		}
		resp.Checks = append(resp.Checks, check)
	}

	resp.Ready = allOK

	status := http.StatusOK
	if !allOK {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error("ready encode error", slog.Any("error", err))
	}
}
