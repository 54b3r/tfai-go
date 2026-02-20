package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// newMetricsTestServer builds a Server backed by a fresh isolated registry so
// tests do not pollute prometheus.DefaultRegisterer.
func newMetricsTestServer(t *testing.T) (*Server, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	s := &Server{
		querier: &fakeQuerier{},
		cfg: &Config{
			ChatTimeout:     5 * time.Minute,
			MetricsRegistry: reg,
			MetricsGatherer: reg,
		},
		metrics: newServerMetrics(reg),
	}
	return s, reg
}

func Test_Metrics_EndpointReturns200(t *testing.T) {
	t.Parallel()
	_, reg := newMetricsTestServer(t)

	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/metrics", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("want text/plain content-type, got %q", ct)
	}
}

func Test_Metrics_ChatCounterIncremented(t *testing.T) {
	t.Parallel()
	s, reg := newMetricsTestServer(t)

	// Simulate a successful chat request via the counter directly.
	s.metrics.chatRequestsTotal.WithLabelValues("ok").Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "tfai_chat_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "outcome" && lp.GetValue() == "ok" {
						if m.GetCounter().GetValue() != 1 {
							t.Errorf("want counter=1, got %v", m.GetCounter().GetValue())
						}
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("tfai_chat_requests_total{outcome=\"ok\"} not found in gathered metrics")
	}
}

func Test_Metrics_ActiveStreamsGauge(t *testing.T) {
	t.Parallel()
	s, reg := newMetricsTestServer(t)

	s.metrics.chatActiveStreams.Inc()
	s.metrics.chatActiveStreams.Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "tfai_chat_active_streams" {
			v := mf.GetMetric()[0].GetGauge().GetValue()
			if v != 2 {
				t.Errorf("want active_streams=2, got %v", v)
			}
			return
		}
	}
	t.Error("tfai_chat_active_streams not found in gathered metrics")
}
