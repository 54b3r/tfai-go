package server

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/54b3r/tfai-go/internal/logging"
)

// defaultRateLimit is the number of requests per second allowed per IP on
// rate-limited endpoints when no explicit limit is configured.
const defaultRateLimit = 10

// defaultRateBurst is the maximum burst size per IP when no explicit burst is
// configured. A burst of 20 allows short spikes without immediate rejection.
const defaultRateBurst = 20

// ipLimiter holds a token-bucket rate limiter and the last time it was seen,
// used to evict stale entries from the limiter map.
type ipLimiter struct {
	// limiter is the per-IP token bucket.
	limiter *rate.Limiter
	// lastSeen is updated on every request from this IP for LRU eviction.
	lastSeen time.Time
}

// rateLimiter is an HTTP middleware that enforces a per-IP token-bucket rate
// limit. Stale IP entries are evicted every minute to bound memory usage.
type rateLimiter struct {
	// mu protects the limiters map.
	mu sync.Mutex
	// limiters maps remote IP to its per-IP state.
	limiters map[string]*ipLimiter
	// rps is the sustained request rate allowed per IP (requests/second).
	rps rate.Limit
	// burst is the maximum instantaneous burst per IP.
	burst int
	// log is the structured logger for rate-limit events.
	log *slog.Logger
}

// newRateLimiter constructs a rateLimiter and starts the background eviction
// goroutine. The goroutine exits when the returned stop function is called.
// rps and burst are the per-IP token-bucket parameters.
func newRateLimiter(rps float64, burst int, log *slog.Logger) (*rateLimiter, func()) {
	rl := &rateLimiter{
		limiters: make(map[string]*ipLimiter),
		rps:      rate.Limit(rps),
		burst:    burst,
		log:      log,
	}

	stopCh := make(chan struct{})
	go rl.evictLoop(stopCh)

	return rl, func() { close(stopCh) }
}

// getLimiter returns the per-IP limiter for the given IP, creating one if
// it does not already exist.
func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &ipLimiter{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

// evictLoop removes IP entries that have not been seen for more than 5 minutes.
// It runs in a background goroutine and exits when stopCh is closed.
func (rl *rateLimiter) evictLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			rl.evict()
		}
	}
}

// evict removes stale IP entries older than 5 minutes.
func (rl *rateLimiter) evict() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	for ip, entry := range rl.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(rl.limiters, ip)
		}
	}
}

// middleware returns an http.Handler that enforces the rate limit before
// delegating to next. Requests that exceed the limit receive 429 Too Many
// Requests with a Retry-After header and a structured WARN log entry.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			log := logging.FromContext(r.Context())
			log.Warn("rate limit exceeded",
				slog.String("ip", ip),
				slog.String("path", r.URL.Path),
			)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the remote IP from the request, stripping the port.
// It does not trust X-Forwarded-For since this server is local-only.
func clientIP(r *http.Request) string {
	addr := r.RemoteAddr
	// RemoteAddr is "host:port" for TCP connections.
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
