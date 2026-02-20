# TF-AI-Go — Unified Roadmap

**Date:** 2026-02-20  
**Current Version:** v0.18.0 (+ PR #39 in-flight)  
**Sources:** [REVIEW.md](./REVIEW.md) | [SRE_ASSESSMENT.md](./SRE_ASSESSMENT.md) | [STRATEGIC_ANALYSIS.md](./STRATEGIC_ANALYSIS.md)

This document consolidates every finding, fix, and feature across all review documents into a single prioritised plan of action. Items are tracked by ID for cross-referencing with GitHub issues and review docs.

---

## Tier 1: Immediate — Get Sharable

**Goal:** Fix every issue that could embarrass you, confuse testers, or create a security/stability risk when another platform engineer runs this against a real Terraform repo.

**Timeline:** 1–2 focused sessions (~4 hours total)  
**Branch:** `fix/pre-release-hardening`  
**Merge target:** v0.19.0

### 1.1 Dead Metrics — Wire or Remove

| | |
|---|---|
| **ID** | MF-1 |
| **Source** | SRE_ASSESSMENT.md §4, REVIEW.md §4.1 |
| **Problem** | `tfai_http_requests_total` and `tfai_http_duration_seconds` are registered in `metrics.go` but never incremented. They show as zeros in `/metrics`, making the observability stack look broken. |
| **Fix** | Create a `metricsMiddleware` in `internal/server/middleware.go` that wraps every route handler. Increment the counter with `method`, `handler`, `code` labels and observe duration on the histogram. Apply it in the mux chain in `server.go`. |
| **Files** | `internal/server/middleware.go`, `internal/server/server.go`, `internal/server/metrics.go` |
| **Effort** | ~30 LOC |
| **Risk if skipped** | Testers see empty metrics, lose confidence in observability. |

### 1.2 Body Size Limits on Workspace Endpoints

| | |
|---|---|
| **ID** | MF-2 / SEC-1 |
| **Source** | SRE_ASSESSMENT.md §2, §5 |
| **Problem** | `/api/chat` has `maxChatBodyBytes = 1 MiB` but `/api/workspace/create` and `/api/file` PUT have no body limits. A multi-GB POST causes OOM. |
| **Fix** | Wrap `r.Body` with `http.MaxBytesReader(w, r.Body, limit)` at the top of `handleWorkspaceCreate` and `handleFileSave`. Use 1 MiB for create, 5 MiB for file save. |
| **Files** | `internal/server/workspace.go` |
| **Effort** | ~5 LOC |
| **Risk if skipped** | OOM crash from a single malformed request. |

### 1.3 Cap Workspace Context Builder

| | |
|---|---|
| **ID** | MF-3 / R1 |
| **Source** | SRE_ASSESSMENT.md §6 |
| **Problem** | `buildWorkspaceContext` reads ALL `.tf` files into memory with no caps. A Terraform monorepo with hundreds of files produces a multi-MB context that blows the token budget and wastes memory. |
| **Fix** | Add constants: `maxWorkspaceFiles = 50`, `maxFileSize = 100 * 1024`, `maxTotalSize = 1024 * 1024`. Skip files exceeding per-file cap, stop walking after total cap, log a warning when limits hit. |
| **Files** | `internal/agent/agent.go` |
| **Effort** | ~20 LOC |
| **Risk if skipped** | OOM or 5-minute timeout on large repos. Tester with a monorepo hits this immediately. |

### 1.4 Request ID Propagation into Agent

| | |
|---|---|
| **ID** | MF-4 / LOG-1 |
| **Source** | SRE_ASSESSMENT.md §3 |
| **Problem** | `handleChat` creates a fresh logger with `session_id` but doesn't inherit the `request_id` from the middleware logger. Logs from the agent layer can't be correlated to the HTTP request. |
| **Fix** | In `handleChat`, use `logging.FromContext(r.Context()).With(slog.String("session_id", ...))` instead of `slog.New(...)`. |
| **Files** | `internal/server/server.go` |
| **Effort** | ~3 LOC |
| **Risk if skipped** | Can't trace a chat request through logs. Debugging production issues is blind. |

### 1.5 Chat Completion Success Log

| | |
|---|---|
| **ID** | LOG-2 |
| **Source** | SRE_ASSESSMENT.md §3 |
| **Problem** | `handleChat` logs `"chat start"` but not `"chat complete"`. Can't confirm successful responses in logs or measure end-to-end duration from logs alone. |
| **Fix** | Add a deferred `log.Info("chat complete", slog.Duration("duration", ...), slog.Bool("files_written", ...))` after the stream completes. |
| **Files** | `internal/server/server.go` |
| **Effort** | ~5 LOC |
| **Risk if skipped** | No log evidence of successful responses. |

### 1.6 CI Pipeline (GitHub Actions)

| | |
|---|---|
| **ID** | REVIEW §7.1 / Phase 1.8 |
| **Source** | REVIEW.md §7 |
| **Problem** | No CI. The gate runs locally only. A PR could be merged without passing any checks. |
| **Fix** | Create `.github/workflows/gate.yml`: checkout → setup-go → install golangci-lint → install govulncheck → `make gate`. Trigger on push and PR to `main`. |
| **Files** | `.github/workflows/gate.yml` |
| **Effort** | ~60 LOC (YAML) |
| **Risk if skipped** | PRs merge without passing gate. First external contributor breaks the build. |

### Tier 1 Summary

| ID | Item | LOC | Files |
|---|---|---|---|
| MF-1 | Wire dead HTTP metrics | ~30 | middleware.go, server.go, metrics.go |
| MF-2 | Body size limits on workspace endpoints | ~5 | workspace.go |
| MF-3 | Cap workspace context builder | ~20 | agent.go |
| MF-4 | Request ID propagation | ~3 | server.go |
| LOG-2 | Chat completion success log | ~5 | server.go |
| CI | GitHub Actions gate | ~60 | gate.yml |

**Total: ~123 LOC across 6 items. One branch, one PR, one afternoon.**

---

## Tier 2: Medium-Term — Production Confidence

**Goal:** Harden the service for sustained use, complete the RAG pipeline so the product has its differentiating feature, and make operational debugging straightforward.

**Timeline:** 2–3 weeks of focused work  
**Branches:** One per logical grouping below  
**Merge target:** v0.20.0 → v0.24.0

### 2.1 Security Hardening

| ID | Item | Source | Effort |
|---|---|---|---|
| SEC-2 | `--workspace-root` flag to confine all file operations | REVIEW.md §1.3, SRE §2 | ~50 LOC |
| SEC-3 | XSS protection — sanitize markdown in chat UI | REVIEW.md §9 | ~20 LOC |
| SEC-4 | Redact sensitive values from `terraform state pull` output | SRE §2 | ~30 LOC |
| REVIEW-1.5 | Fix `writeJSONError` potential JSON injection | REVIEW.md §1.5 | ~5 LOC |
| REVIEW-1.6 | Move docker-compose secrets to `.env` file | REVIEW.md §1.6 | Config |
| REVIEW-1.7 | Document threat model (STRIDE or equivalent) | REVIEW.md §1.7 | Prose |

### 2.2 Observability & Debugging

| ID | Item | Source | Effort |
|---|---|---|---|
| SF-1 | pprof debug port (`--debug-port` flag, localhost only) | SRE §1 | ~15 LOC |
| SF-5 | Log resolved config at startup (non-secret values) | SRE §8 | ~15 LOC |
| LOG-3 | Log RAG retrieval success (N docs injected) | SRE §3 | ~3 LOC |
| LOG-4 | Log history load/trim at Info level | SRE §3 | ~5 LOC |
| OBS-1 | Business metrics: `tfai_tool_invocations_total{tool,outcome}` | SRE §4 | ~20 LOC |
| OBS-2 | Business metrics: `tfai_rag_documents_injected`, `tfai_history_messages_trimmed` | SRE §4 | ~15 LOC |
| OBS-3 | Trace ID from Langfuse in structured logs | SRE §4 | ~10 LOC |

### 2.3 Resilience & Resource Management

| ID | Item | Source | Effort |
|---|---|---|---|
| SF-2 | Terraform command execution timeout (2 min deadline) | SRE §5 | ~10 LOC |
| SF-3 | Replace LLMPinger full-generate with lightweight/cached check | SRE §7 | ~30 LOC |
| SF-4 | Read `QDRANT_PORT` env var in `buildPingers` | SRE §7 | ~3 LOC |
| SF-6 | Send `event: error` to active SSE streams on SIGTERM | SRE §5 | ~20 LOC |
| NH-1 | Max concurrent chat streams semaphore | SRE §5 | ~15 LOC |
| NH-2 | Cap response buffer in `agent.Query()` (1 MB) | SRE §6 | ~10 LOC |
| SCALE-1 | Include hostname/pod in session ID for multi-replica dedup | STRATEGIC §Part 1 | ~5 LOC |

### 2.4 Test Coverage

| ID | Item | Source | Effort |
|---|---|---|---|
| TEST-1 | Unit tests for `internal/tools` (plan, state, generate) | REVIEW.md §2.1 | ~200 LOC |
| TEST-2 | Unit tests for `agent.buildMessages()` variations | REVIEW.md §2.2 | ~150 LOC |
| TEST-3 | Fuzz test for `parseAgentOutput()` | REVIEW.md §2.5 | ~30 LOC |
| TEST-4 | Integration test suite (`//go:build integration`) | REVIEW.md §2.4 | ~300 LOC |

### 2.5 RAG Pipeline — Make It Work

| ID | Item | Source | Effort | Depends On |
|---|---|---|---|---|
| RAG-1 | RAG architecture review (ADR) | #36 | Prose | — |
| RAG-2 | Implement `Embedder` for Ollama + OpenAI | #34 | ~200 LOC | RAG-1 |
| RAG-3 | Fix `upsertWithEmbeddings` to attach vectors to Qdrant docs | REVIEW.md §3.4 | ~20 LOC | RAG-2 |
| RAG-4 | Wire `ingest` CLI end-to-end (fetch → chunk → embed → upsert) | REVIEW.md §3.3 | ~100 LOC | RAG-3 |
| RAG-5 | Semantic chunking for HCL/Terraform documentation | REVIEW.md §3.5 | ~200 LOC | RAG-4 |

### 2.6 MCP Spike

| ID | Item | Source | Effort |
|---|---|---|---|
| MCP-1 | 2-hour spike: expose `terraform_plan` as MCP tool via `mcp-go` | STRATEGIC §Option C | ~100 LOC |
| MCP-2 | If spike succeeds: full MCP server branch with plan + state + generate | STRATEGIC §Option C | ~300 LOC |

### Tier 2 Suggested Execution Order

```
Branch 1: fix/observability-logging       → 2.2 items (SF-1, SF-5, LOG-3, LOG-4, OBS-1–3)
Branch 2: fix/resilience-resource-mgmt    → 2.3 items (SF-2–6, NH-1–2, SCALE-1)
Branch 3: fix/security-hardening          → 2.1 items (SEC-2–4, REVIEW-1.5–1.7)
Branch 4: feat/test-coverage              → 2.4 items (TEST-1–4)
Branch 5: feat/rag-pipeline               → 2.5 items (RAG-1–5, sequential)
Branch 6: spike/mcp-server                → 2.6 items (MCP-1, then MCP-2 if viable)
```

**RAG and MCP are the product decisions.** Everything else is hardening. Do branches 1–3 first (operational confidence), then branch 5 or 6 depending on whether RAG or MCP is the higher-value path. Tests (branch 4) can be done incrementally alongside other work.

---

## Tier 3: Complete — Full Product Maturity

**Goal:** Everything needed for a v1.0 release, enterprise deployment readiness, and open-source sustainability.

**Timeline:** 2–3 months, sequenced after Tier 2  
**Prerequisites:** Tiers 1 and 2 complete, RAG pipeline working, MCP decision made

### 3.1 Enterprise Deployment

| ID | Item | Source | Effort |
|---|---|---|---|
| K8S-1 | Helm chart (deployment, service, configmap, secret, ingress) | STRATEGIC §Part 3 | ~300 LOC |
| K8S-2 | Network policy for pod-to-pod traffic | STRATEGIC §Part 3 | ~50 LOC |
| K8S-3 | Startup probe (longer timeout for slow LLM init) | SRE §7 | ~10 LOC |
| K8S-4 | Multi-arch Dockerfile (`GOARCH` from `--platform`) | REVIEW.md §4.6 | ~10 LOC |
| SBOM-1 | Generate SBOM artifact in CI (syft or trivy) | STRATEGIC §Part 3, SRE §2 | ~30 LOC |
| RBAC-1 | Multi-user auth (JWT or OIDC) with user identity in logs | STRATEGIC §Part 3 | ~200 LOC |
| AUDIT-1 | Structured audit log (who did what, when, to which workspace) | STRATEGIC §Part 3 | ~100 LOC |
| TLS-1 | TLS support or reverse proxy documentation | REVIEW.md §4.2 | ~30 LOC |

### 3.2 Release & CI/CD

| ID | Item | Source | Effort |
|---|---|---|---|
| REL-1 | Release automation (goreleaser) | REVIEW.md §4.5 | ~100 LOC |
| REL-2 | Changelog generation (conventional-changelog or release-please) | New | ~50 LOC |
| REL-3 | Container image push to GHCR in CI | New | ~30 LOC |
| DASH-1 | Grafana dashboard JSON (request rate, latency, errors, active streams) | SRE §4 | ~200 LOC |
| ALERT-1 | Prometheus alerting rules YAML | SRE §4 | ~50 LOC |

### 3.3 UI & UX

| ID | Item | Source | Effort |
|---|---|---|---|
| UI-1 | Vite + React migration (only if MCP is NOT the primary path) | #12 | ~2000 LOC |
| UI-2 | SSE reconnection logic on disconnect/server restart | STRATEGIC §Part 1 | ~50 LOC |
| UI-3 | Dark mode, responsive layout, accessibility | New | ~300 LOC |

### 3.4 Advanced RAG (after basic RAG works)

| ID | Item | Source | Effort | Depends On |
|---|---|---|---|---|
| RAG-6 | Reranking pipeline (cross-encoder or Cohere rerank) | #35 | ~300 LOC | RAG-5 |
| RAG-7 | RAG evaluation framework (recall@k, MRR) | #36 | ~200 LOC | RAG-5 |
| RAG-8 | Hybrid search (sparse + dense vectors in Qdrant) | New | ~150 LOC | RAG-5 |
| RAG-9 | Incremental re-ingestion (detect changed docs, update only deltas) | New | ~200 LOC | RAG-4 |

### 3.5 Developer Experience

| ID | Item | Source | Effort |
|---|---|---|---|
| DX-1 | 3 Musketeers dev container (Docker-based dev env) | #10 | ~200 LOC |
| DX-2 | Hot-reload dev server (`.air.toml`) | #11 | ~30 LOC |
| DX-3 | Contributing guide (CONTRIBUTING.md) | New | Prose |
| DX-4 | Architecture decision records (ADR directory) | New | Prose |
| README-1 | Update README with architecture diagram, quickstart, screenshots | REVIEW.md | Prose |

### 3.6 Strategic Options (choose based on Tier 2 learnings)

| ID | Item | Source | Decision Point |
|---|---|---|---|
| OPT-A | Ship v1.0 as standalone product | STRATEGIC §Option A | Default path |
| OPT-B | Extract project template repo (cookiecutter) | STRATEGIC §Option B | After v1.0, if building product #2 |
| OPT-C | Full MCP server (replace standalone server) | STRATEGIC §Option C | If MCP spike succeeds in Tier 2 |

---

## Cross-Reference: Items Already Completed

| ID | Item | Version | PR |
|---|---|---|---|
| ~~#22~~ | Token budget management | v0.18.0 | #38 |
| ~~S1~~ | Constant-time token comparison | PR #39 | #39 |
| ~~S3~~ | Tool invocation audit logging | PR #39 | #39 |
| ~~qa1~~ | govulncheck in `make gate` | PR #39 | #39 |
| ~~SRE~~ | SRE readiness assessment | PR #39 | #39 |
| ~~REVIEW~~ | Codebase review + scorecard | v0.18.0 | #38 |
| ~~STRATEGIC~~ | Strategic analysis + accelerator assessment | v0.18.0 | #38 |
| ~~#21~~ | Conversation history (SQLite) | v0.17.0 | #37 |

---

## GitHub Issues to Create

The following items don't yet have GitHub issues. Create them when starting the corresponding tier:

### For Tier 1 (create now)
- [ ] Pre-release hardening: dead metrics, body limits, workspace caps, request_id, CI

### For Tier 2 (create when starting)
- [ ] pprof debug port
- [ ] Terraform command execution timeout
- [ ] LLMPinger lightweight replacement
- [ ] Workspace root confinement (`--workspace-root`)
- [ ] XSS protection in chat UI
- [ ] Threat model documentation
- [ ] MCP server spike
- [ ] Unit tests for `internal/tools`
- [ ] Integration test suite

### For Tier 3 (create when starting)
- [ ] Helm chart
- [ ] goreleaser automation
- [ ] Multi-user auth (JWT/OIDC)
- [ ] Grafana dashboard + alerting rules

---

## Decision Log

| Date | Decision | Rationale | Source |
|---|---|---|---|
| 2026-02-20 | Do NOT build accelerator/framework | Rule of Three — only 1 domain impl exists. Eino already provides the agent framework. | STRATEGIC_ANALYSIS.md |
| 2026-02-20 | Do NOT build plugin architecture | Over-engineering trap. Build concrete tools. | STRATEGIC_ANALYSIS.md |
| 2026-02-20 | Defer UI migration (#12) | MCP spike may eliminate the need for a custom UI entirely. | STRATEGIC_ANALYSIS.md |
| 2026-02-20 | Defer reranking (#35) | Basic RAG doesn't work yet. Fix fundamentals first. | STRATEGIC_ANALYSIS.md |
| 2026-02-20 | Finish the product (Option A) | Ship v1.0, get real feedback, then decide on template repo or MCP. | STRATEGIC_ANALYSIS.md |
