// Package server — metrics.go registers all Prometheus metrics for the HTTP
// server and exposes helpers used by handlers and middleware.
package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metric label values shared across registrations.
const (
	// labelHandler is the "handler" label value used to partition metrics by
	// the logical endpoint name rather than the raw URL path.
	labelHandler = "handler"
)

// serverMetrics holds all Prometheus metrics owned by the HTTP server.
// A single instance is created in New and stored on Server so that tests can
// inject a fresh prometheus.Registry without polluting the default one.
type serverMetrics struct {
	// chatRequestsTotal counts completed /api/chat requests, partitioned by
	// outcome: "ok", "timeout", or "error".
	chatRequestsTotal *prometheus.CounterVec

	// chatDurationSeconds records the wall-clock duration of each /api/chat
	// request from first byte received to stream completion.
	chatDurationSeconds *prometheus.HistogramVec

	// chatActiveStreams is the number of /api/chat SSE streams currently open.
	chatActiveStreams prometheus.Gauge

	// httpRequestsTotal counts all HTTP requests handled by the mux,
	// partitioned by method, path pattern, and status code.
	httpRequestsTotal *prometheus.CounterVec

	// httpDurationSeconds records the latency of all HTTP requests.
	httpDurationSeconds *prometheus.HistogramVec
}

// newServerMetrics registers all server metrics against reg and returns the
// populated serverMetrics. promauto.With(reg) is used so that each call
// registers into the provided registry rather than the global default —
// this keeps unit tests hermetic.
func newServerMetrics(reg prometheus.Registerer) *serverMetrics {
	factory := promauto.With(reg)

	return &serverMetrics{
		chatRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tfai",
			Subsystem: "chat",
			Name:      "requests_total",
			Help:      "Total number of /api/chat requests completed, partitioned by outcome.",
		}, []string{"outcome"}),

		chatDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "tfai",
			Subsystem: "chat",
			Name:      "duration_seconds",
			Help:      "Wall-clock duration of /api/chat requests from receipt to stream completion.",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		}, []string{"outcome"}),

		chatActiveStreams: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "tfai",
			Subsystem: "chat",
			Name:      "active_streams",
			Help:      "Number of /api/chat SSE streams currently open.",
		}),

		httpRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tfai",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests handled by the server, partitioned by method, handler, and status code.",
		}, []string{"method", labelHandler, "code"}),

		httpDurationSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "tfai",
			Subsystem: "http",
			Name:      "duration_seconds",
			Help:      "Latency of HTTP requests handled by the server.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", labelHandler}),
	}
}
