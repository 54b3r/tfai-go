# TF-AI-Go — SRE Readiness Assessment

**Date:** 2026-02-20  
**Version:** v0.18.0 + fix/security-auth-tool-logging (in-flight)  
**Scope:** Production readiness from an SRE lens — profiling, security, logging, observability, resilience, resource management

---

## Verdict

**Deployment-ready for internal platform engineering use? Yes, with caveats.**  
**Deployment-ready for multi-tenant or internet-facing? No — not without the items in the "Must Fix" section.**

The service handles the SRE basics well (structured logging, health probes, metrics, graceful shutdown, rate limiting). It falls short on profiling, resource bounding, and some observability wiring. This assessment assumes the deployment target is a single-replica k8s pod behind an ingress, serving a small team of platform engineers.

---

## 1. Profiling

### Current State: NOT AVAILABLE

There is **no `pprof` endpoint** exposed. This means:
- You cannot capture CPU or memory profiles from a running instance
- You cannot inspect goroutine stacks to diagnose hangs or leaks
- You cannot trace contention on mutexes or blocking calls

### What's Needed

Expose `net/http/pprof` on a separate listener (NOT the public API port) or behind a debug flag:

```go
// Minimal: register on the default mux behind a flag or internal port
import _ "net/http/pprof"
go http.ListenAndServe("localhost:6060", nil) // debug port, internal only
```

**Endpoints this unlocks:**
- `/debug/pprof/profile` — CPU profile (30s default)
- `/debug/pprof/heap` — heap memory snapshot
- `/debug/pprof/goroutine` — goroutine stack dump
- `/debug/pprof/mutex` — mutex contention
- `/debug/pprof/block` — blocking profile

### Recommendation

| # | Action | Priority | Effort |
|---|---|---|---|
| P1 | Add `--debug-port` flag to `serve` that starts pprof on localhost only | High | ~15 LOC |

---

## 2. Security-First Review

### What's Done Right

| Control | Status | Evidence |
|---|---|---|
| **Constant-time token comparison** | ✅ Fixed | `crypto/subtle.ConstantTimeCompare` in `auth.go` (this PR) |
| **Bearer token auth** | ✅ | `authMiddleware` with `WWW-Authenticate` challenge |
| **Token never logged** | ✅ | Only `token_present: true/false` in log lines |
| **Path traversal prevention** | ✅ | `confineToDir()` with separator-aware prefix check in 3 locations |
| **Body size cap on chat** | ✅ | `maxChatBodyBytes = 1 MiB` |
| **Non-root container** | ✅ | `USER tfai` in Dockerfile |
| **gosec linter** | ✅ | Enabled in `.golangci.yml` |
| **Tool invocation audit logging** | ✅ Fixed | Structured slog before every `terraform` exec (this PR) |
| **CVE scanning** | ✅ Fixed | `govulncheck` added to `make gate` (this PR) |
| **Secrets from environment only** | ✅ | All API keys read from env vars, never hardcoded |

### Remaining Gaps

| # | Gap | Severity | Notes |
|---|---|---|---|
| SEC-1 | **No body size limit on `/api/workspace/create` and `/api/file` (PUT)** | Medium | An authenticated client could POST a multi-GB JSON body, causing OOM. Add `http.MaxBytesReader` to both handlers. |
| SEC-2 | **Workspace root is caller-supplied** | Medium | `handleChat`, `handleFileRead`, `handleFileSave` all accept `workspaceDir` from the request. An authenticated user can read/write any path the process can access. Mitigate with a `--workspace-root` flag that confines all operations. |
| SEC-3 | **No CORS preflight handler** | Low | The server checks `Origin` on responses but doesn't handle `OPTIONS` preflight. Browsers making cross-origin requests will get 405. For local-only this is fine; for k8s behind ingress it may break. |
| SEC-4 | **`terraform state pull` returns raw state** | Medium | Raw state may contain sensitive outputs (passwords, connection strings). The tool returns it directly to the LLM, which may echo it to the UI. Consider redacting sensitive state values. |
| SEC-5 | **No prompt injection guardrails** | Informational | User messages are passed directly to the LLM with no sanitization. The LLM could be tricked into calling `terraform_plan` in a malicious directory. Tracked as known risk. |

---

## 3. Logging Assessment

### What's Done Right

| Pattern | Status | Evidence |
|---|---|---|
| **Structured logging (slog)** | ✅ | JSON handler for production, text for dev |
| **Context-propagated logger** | ✅ | `logging.WithLogger(ctx, log)` / `logging.FromContext(ctx)` |
| **Request ID on every request** | ✅ | `newRequestID()` → 16-byte hex in `requestLogger` middleware |
| **Configurable log level** | ✅ | `LOG_LEVEL` env var (debug/info/warn/error) |
| **Error paths log cause** | ✅ | Consistently `slog.Any("error", err)` on every error |
| **Startup configuration logged** | ✅ | Provider, auth status, history store path, Langfuse status |
| **Tool invocations logged** | ✅ | Subcommand + args + workspace dir (this PR) |

### Gaps

| # | Gap | Impact | Fix |
|---|---|---|---|
| LOG-1 | **`request_id` not propagated to agent** | Can't trace a chat request through the LLM call | `handleChat` creates a new logger with `session_id` but doesn't inherit the `request_id` from the middleware logger. Fix: use `logging.FromContext(r.Context()).With(slog.String("session_id", ...))` instead of creating a fresh logger. |
| LOG-2 | **No log on chat completion (success path)** | Can't confirm successful responses in logs | `handleChat` logs `"chat start"` but not `"chat complete"` with duration and token count. Add a deferred log after the stream completes. |
| LOG-3 | **RAG retrieval not logged** | Can't tell if RAG was used for a given request | `buildMessages` logs a warning on RAG failure but nothing on success. Add `log.Info("rag: injected N documents")`. |
| LOG-4 | **History load/trim not logged at Info level** | Can't confirm history is working | History load failures are warned, but successful loads (N messages loaded, M trimmed) are not logged. |
| LOG-5 | **No log rotation or size cap** | Disk fills on long-running instances | The logger writes to stderr with no rotation. In k8s this is fine (stdout/stderr captured by the runtime). For bare-metal, recommend using `logrotate` or documenting the assumption. |

### Log Maturity Rating: **7/10**
Structured, context-propagated, request-scoped. Needs better success-path logging and request_id propagation into the agent layer.

---

## 4. Observability Stack Assessment

### Metrics

| Metric | Registered | Incremented | Status |
|---|---|---|---|
| `tfai_chat_requests_total{outcome}` | ✅ | ✅ | Working — counts ok/error/timeout |
| `tfai_chat_duration_seconds{outcome}` | ✅ | ✅ | Working — histogram with 1s–5m buckets |
| `tfai_chat_active_streams` | ✅ | ✅ | Working — gauge, inc/dec around stream |
| `tfai_http_requests_total{method,handler,code}` | ✅ | ❌ | **DEAD** — registered but never incremented |
| `tfai_http_duration_seconds{method,handler}` | ✅ | ❌ | **DEAD** — registered but never incremented |

**2 of 5 metric families are dead code.** The `httpRequestsTotal` and `httpDurationSeconds` were registered in `metrics.go` but no middleware calls `Observe()` or `Inc()` on them. They will always be zero in `/metrics` output.

### Missing Metrics That Matter

| Metric | Why |
|---|---|
| `tfai_history_messages_loaded` | Confirms history is being used |
| `tfai_history_messages_trimmed` | Shows budget pressure |
| `tfai_rag_documents_injected` | Confirms RAG is contributing |
| `tfai_tool_invocations_total{tool,outcome}` | Shows which tools the LLM is calling |
| `tfai_workspace_files_written_total` | Tracks code generation activity |
| Process metrics (goroutines, memory, GC) | `promhttp.InstrumentMetricHandler` or `collectors.NewGoCollector()` — may already be registered via default registry |

### Tracing

| Aspect | Status |
|---|---|
| **Langfuse integration** | ✅ Opt-in, per-request sessions |
| **Span-level tracing** | Partial — Eino provides model-level spans but tool calls are not individually traced |
| **Trace ID in logs** | ❌ — `session_id` is logged but not Langfuse trace ID |
| **W3C Trace Context / OpenTelemetry** | ❌ — not supported. Langfuse is the only tracing backend. |

### Alerting & Dashboards

None. No `alerts.yml`, no Grafana dashboard JSON, no runbook. For internal use this is acceptable if you monitor `/metrics` manually. For production, you need at minimum:

- **Alert:** `tfai_chat_active_streams > 10 for 5m` (goroutine leak or hung backends)
- **Alert:** `rate(tfai_chat_requests_total{outcome="error"}[5m]) > 0.1` (error spike)
- **Alert:** `rate(tfai_chat_requests_total{outcome="timeout"}[5m]) > 0` (backend timeouts)
- **Dashboard:** Chat request rate, duration p50/p95/p99, active streams, error rate

### Observability Maturity Rating: **5/10**
Foundation is solid (Prometheus + Langfuse). Dead metrics, missing business metrics, and no alerting rules keep it from being operationally useful.

---

## 5. Resilience & Fault Tolerance

### Timeouts

| Path | Timeout | Set? | Notes |
|---|---|---|---|
| HTTP read | 30s | ✅ | `ReadTimeout` on `http.Server` |
| HTTP write | 5m | ✅ | `WriteTimeout` — long for SSE streaming |
| Chat LLM call | 5m | ✅ | `context.WithTimeout` in `handleChat` |
| Readiness probe per-dependency | 5s | ✅ | `probeTimeout` in `handleReady` |
| Graceful shutdown | 10s | ✅ | `ShutdownTimeout` |
| HTTP fetch (ingestion) | 30s | ✅ | `http.Client.Timeout` in pipeline |
| Terraform command execution | ❌ | ❌ | **No timeout on `ExecRunner.Run()`** — a hung `terraform plan` will block until the parent context is cancelled (chat timeout or process kill). Should add a tool-specific deadline. |
| SQLite operations | ❌ | ❌ | **No timeout on store queries** — `busy_timeout=5000` is set on the driver but there's no `context.WithTimeout` on individual operations. A stuck write could block the request handler. |

### Graceful Degradation

| Dependency | Failure Behaviour | Correct? |
|---|---|---|
| LLM provider | `Query()` returns error → SSE error event | ✅ |
| RAG retriever | Warning logged, query continues without context | ✅ |
| Conversation store (history) | Warning logged, query continues stateless | ✅ |
| Terraform binary missing | Warning at startup, tools excluded from agent | ✅ |
| Qdrant unreachable | Readiness probe fails (503), service stays alive | ✅ |
| SQLite store open fails | Warning logged, history disabled | ✅ |

**Graceful degradation is excellent.** Every optional dependency fails soft with a warning and the core chat path stays alive.

### Shutdown Sequence

```
SIGTERM → signal.NotifyContext → ctx.Done() 
  → http.Server.Shutdown(10s) → drain in-flight requests
    → stopRL() → stop rate limiter eviction goroutine
      → hs.Close() → close SQLite store (deferred)
        → flush() → flush Langfuse traces (deferred)
```

**Issue:** `ShutdownTimeout` (10s) vs `ChatTimeout` (5m). If a chat stream is mid-flight when SIGTERM arrives, `Shutdown` waits 10 seconds then kills the connection. The client gets a broken SSE stream with no `event: done`. The assistant message won't be persisted (history `Append` happens after `Query` returns).

**Fix:** Either increase `ShutdownTimeout` to match `ChatTimeout`, or on SIGTERM send `event: error data: server shutting down` to active streams and drain them.

### Backpressure

| Mechanism | Present? | Notes |
|---|---|---|
| Rate limiter (per-IP token bucket) | ✅ | 10 req/s sustained, 20 burst |
| Chat body size cap | ✅ | 1 MiB |
| Max concurrent streams | ❌ | No limit — 1000 simultaneous chat requests would create 1000 goroutines, each holding an LLM connection, an SSE stream, and a strings.Builder that grows until the response completes. |
| File size caps on read/write | ❌ | `handleFileRead` uses `os.ReadFile` — a 10 GB file would OOM. `handleFileSave` has no body limit. |
| Workspace walk depth | ❌ | `buildWorkspaceContext` walks the entire directory tree and reads every `.tf` file into memory. A monorepo with thousands of files could produce a multi-MB context. |

---

## 6. Resource Management

### Memory

| Source | Bounded? | Risk |
|---|---|---|
| Chat response buffer (`strings.Builder` in `Query`) | ❌ | Grows unbounded until LLM stops generating. A hallucinating model could produce MBs. |
| Workspace context (`buildWorkspaceContext`) | ❌ | Reads ALL `.tf` files into memory. No file count or size cap. |
| Rate limiter map | ✅ | Evicted every 5 minutes. Bounded by unique IPs × entry size. |
| SSE writer | ✅ | Streams to the HTTP response, doesn't accumulate. |

### Goroutines

| Source | Bounded? | Notes |
|---|---|---|
| HTTP handler goroutines | ❌ | One per request, no limit. `net/http` default. |
| Rate limiter eviction | ✅ | Single goroutine, stopped on shutdown. |
| Langfuse callback flusher | ✅ | Managed by Langfuse SDK. |

### File Descriptors

| Source | Notes |
|---|---|
| HTTP connections | Bounded by OS limits. No explicit `MaxOpenConns`. |
| SQLite connection | Single connection with WAL mode. Correct. |
| Qdrant gRPC | Single gRPC channel. Correct. |

### Recommendation

| # | Action | Priority | Effort |
|---|---|---|---|
| R1 | Cap `buildWorkspaceContext` — max 50 files, max 100 KB per file, max 1 MB total | High | ~20 LOC |
| R2 | Cap `strings.Builder` in `Query` — if response exceeds 1 MB, truncate and log warning | Medium | ~10 LOC |
| R3 | Add `http.Server.MaxHeaderBytes` (default is 1 MB, which is fine) | Low | 1 line |

---

## 7. Health Probes (k8s readiness)

### Liveness: `GET /api/health`
Returns `{"status":"ok"}` with 200. Simple, correct. No dependencies checked. This is correct for liveness — the process is alive if it can respond.

### Readiness: `GET /api/ready`
Iterates registered `Pinger` implementations:
- **LLMPinger**: sends `schema.UserMessage("ping")` → `model.Generate()` → waits for full response
- **QdrantPinger**: calls `client.HealthCheck()` gRPC

### Issues

| # | Issue | Impact |
|---|---|---|
| HP-1 | **LLMPinger sends a full generate request** | Consumes tokens on paid APIs. At 10s k8s probe interval → 8,640 LLM calls/day → ~$2-8/day on GPT-4o just for health checks. |
| HP-2 | **No startup probe** | K8s may mark the pod as failed during slow LLM provider initialization. Add a startup probe with a longer timeout, or delay readiness until the first successful LLM ping. |
| HP-3 | **Qdrant port hardcoded to 6334** | `buildPingers` ignores `QDRANT_PORT` env var. The docker-compose exposes both 6333 and 6334. |
| HP-4 | **SQLite store health not probed** | If the history DB is corrupted or disk-full, readiness doesn't reflect it. |

### Recommendation

| # | Action | Priority | Effort |
|---|---|---|---|
| HP-1 | Replace LLMPinger with a lightweight check (list models API for Ollama, or cache first ping result for 60s) | High | ~30 LOC |
| HP-3 | Read `QDRANT_PORT` env var in `buildPingers` | Low | ~3 LOC |

---

## 8. Configuration & Environment Variables

### Current Variables (from `.env.example` + code)

| Variable | Required | Default | Validated | Logged at startup |
|---|---|---|---|---|
| `MODEL_PROVIDER` | No | `ollama` | ✅ `Validate()` | ✅ |
| `OLLAMA_HOST` | No | `http://localhost:11434` | No | No |
| `OLLAMA_MODEL` | If ollama | `llama3` | ✅ | No |
| `OPENAI_API_KEY` | If openai | — | ✅ | No (correct) |
| `OPENAI_MODEL` | If openai | `gpt-4o` | ✅ | No |
| `AZURE_OPENAI_*` | If azure | varies | ✅ | No |
| `BEDROCK_MODEL_ID` | If bedrock | — | ✅ | No |
| `GOOGLE_API_KEY` | If gemini | — | ✅ | No (correct) |
| `MODEL_MAX_TOKENS` | No | `4096` | Parsed, falls back | No |
| `MODEL_TEMPERATURE` | No | `0.2` | Parsed, falls back | No |
| `TFAI_API_KEY` | No | — (auth disabled) | N/A | ✅ (presence only) |
| `TFAI_HISTORY_DB` | No | `~/.tfai/history.db` | ✅ | ✅ |
| `QDRANT_HOST` | No | — (probes skip) | No | Implicit |
| `LOG_LEVEL` | No | `info` | ✅ | No |
| `LOG_FORMAT` | No | `json` | ✅ | No |
| `LANGFUSE_*` | No | — (tracing disabled) | Presence check | ✅ |

### Issues

- **No startup config dump**: The service logs provider and auth status but not the full resolved configuration. For debugging, a single Info log with all non-secret config values at startup would help.
- **`MODEL_MAX_TOKENS` and `MODEL_TEMPERATURE` silently fall back**: If you set `MODEL_MAX_TOKENS=abc`, it silently uses `4096`. Should log a warning.
- **No config validation on `QDRANT_PORT`**: The value is read in docker-compose but hardcoded in Go. 

---

## 9. SRE Checklist — Final Scorecard

| # | Control | Status | Notes |
|---|---|---|---|
| 1 | Structured logging | ✅ | slog, JSON, context-propagated |
| 2 | Request ID on every request | ✅ | 16-byte hex, injected in middleware |
| 3 | Request ID in all log lines | ⚠️ | Lost when entering agent layer |
| 4 | Liveness probe | ✅ | `/api/health` |
| 5 | Readiness probe | ✅ | `/api/ready` with per-dependency checks |
| 6 | Graceful shutdown | ✅ | Signal → drain → close |
| 7 | Timeouts on all outbound calls | ⚠️ | Missing on terraform exec and SQLite |
| 8 | Secrets never logged | ✅ | Confirmed in all log paths |
| 9 | Secrets from env only | ✅ | No hardcoded keys |
| 10 | Rate limiting | ✅ | Per-IP token bucket |
| 11 | Body size limits | ⚠️ | Only on `/api/chat` |
| 12 | Prometheus metrics | ⚠️ | 2/5 families dead, no business metrics |
| 13 | Profiling endpoint | ❌ | No pprof |
| 14 | CVE scanning | ✅ | govulncheck in gate (this PR) |
| 15 | Audit trail for tool calls | ✅ | Structured log before every terraform exec (this PR) |
| 16 | Graceful degradation | ✅ | All optional deps fail soft |
| 17 | Input validation | ✅ | Path traversal, body parsing, required fields |
| 18 | Resource bounding | ⚠️ | No caps on workspace context or response buffer |
| 19 | CI pipeline | ❌ | No GitHub Actions |
| 20 | Alerting rules | ❌ | No alert definitions |

**Pass: 12/20 | Partial: 5/20 | Fail: 3/20**

---

## 10. Prioritised Fix List for Pre-Release

### Must Fix (before sharing with other engineers)

| # | Item | Effort | Risk if skipped |
|---|---|---|---|
| MF-1 | Wire `httpRequestsTotal` + `httpDurationSeconds` in middleware (or remove dead metrics) | ~30 LOC | Confusing `/metrics` output, looks broken |
| MF-2 | Add body size limits to `/api/workspace/create` and `/api/file` PUT | ~5 LOC | OOM on malicious input |
| MF-3 | Cap `buildWorkspaceContext` (50 files, 100KB/file, 1MB total) | ~20 LOC | OOM on large monorepos |
| MF-4 | Fix `request_id` propagation into agent (use `logging.FromContext(r.Context())`) | ~3 LOC | Can't trace requests end-to-end |

### Should Fix (before production)

| # | Item | Effort |
|---|---|---|
| SF-1 | Add pprof debug port (`--debug-port` flag) | ~15 LOC |
| SF-2 | Add tool execution timeout (e.g. 2 minute deadline on terraform commands) | ~10 LOC |
| SF-3 | Replace LLMPinger full-generate with lightweight check or cached result | ~30 LOC |
| SF-4 | Read `QDRANT_PORT` env var in `buildPingers` | ~3 LOC |
| SF-5 | Log resolved config at startup (non-secret values) | ~15 LOC |
| SF-6 | Add `event: error` on SIGTERM for active SSE streams | ~20 LOC |

### Nice to Have (before scaling)

| # | Item | Effort |
|---|---|---|
| NH-1 | Max concurrent chat streams semaphore | ~15 LOC |
| NH-2 | Cap response buffer in agent.Query() | ~10 LOC |
| NH-3 | GitHub Actions CI pipeline | ~100 LOC |
| NH-4 | Grafana dashboard JSON + alert rules YAML | ~200 LOC |
| NH-5 | Helm chart for k8s deployment | ~300 LOC |

---

## 11. Overall SRE Readiness Rating

**6.5/10** — Good foundation. Logging, health probes, graceful degradation, and rate limiting are solid. The gaps are in profiling, resource bounding, metrics completeness, and the terraform execution timeout. The "Must Fix" list is ~60 LOC total and can be done in an afternoon.

For sharing with a small team of trusted platform engineers on an internal network, **this is ready after the Must Fix items**. For internet-facing or multi-tenant, it needs the "Should Fix" list plus the security items from REVIEW.md (workspace root confinement, RBAC).
