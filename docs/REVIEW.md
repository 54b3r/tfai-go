# TF-AI-Go — Codebase Review & Roadmap

**Date:** 2026-02-20  
**Version:** v0.17.0 (tag) + in-flight `feat/22-token-budget-management`  
**Reviewer:** Cascade (AI pair programmer)  
**Scope:** Full repository — architecture, code quality, security, testing, operations

---

## 1. Executive Summary

TF-AI-Go is a local-first AI assistant for Terraform engineers. It wraps multiple LLM backends (Ollama, OpenAI, Azure, Bedrock, Gemini) behind a ReAct agent with tool-calling, RAG retrieval, and a web UI. The codebase is **well-structured for its maturity** — clean package boundaries, consistent error handling, structured logging, and a strict pre-commit gate. However, several areas need attention before this could pass a security audit or be considered production-grade.

### Scorecard

| Area | Score | Notes |
|---|---|---|
| **Architecture** | 7/10 | Clean layering, good interface use. Agent is monolithic. |
| **Code Quality** | 8/10 | Consistent style, doc comments, golangci-lint enforced. |
| **Security** | 5/10 | Path traversal guards exist but gaps remain. No TLS. Token comparison is not constant-time. |
| **Testing** | 5/10 | Server package well-tested. 7 packages have zero tests. No integration tests. |
| **Observability** | 7/10 | Structured logging, Langfuse tracing, Prometheus metrics. Missing request_id propagation to agent. |
| **Operations** | 6/10 | Dockerfile, docker-compose, Makefile. No CI pipeline. No govulncheck. |
| **Documentation** | 7/10 | Good README, SMOKE_TEST, .env.example. No ADRs, no API reference. |

**Overall: 6.4/10** — Solid foundation for a pre-alpha project. Not audit-ready.

---

## 2. Architecture

### What's Good

- **Clean package boundaries**: `cmd/` → `internal/agent` → `internal/server` → `internal/provider` → `internal/rag` → `internal/store`. No circular dependencies.
- **Interface-driven design**: `querier`, `Runner`, `Pinger`, `ConversationStore`, `Retriever`, `Embedder`, `VectorStore` — all testable via fakes.
- **Provider factory pattern**: `provider.New()` + `provider.NewFromEnv()` cleanly separates config resolution from construction.
- **Middleware chain**: `requestLogger → authMiddleware → rateLimiter → handler` is correct and composable.

### What Needs Work

- **Agent is a god object**: `agent.go` handles message building, history injection, RAG context, workspace context, streaming, file parsing, file writing, and budget trimming. This is ~340 lines and growing. Should be decomposed into a pipeline of message builders.
- **No dependency injection container**: `serve.go` manually wires 8+ dependencies in a 50-line RunE closure. This will get worse as features are added. Consider a `wire` or manual DI struct.
- **CLI commands duplicate agent construction**: `ask.go`, `generate.go`, `diagnose.go` each independently construct the provider + tools + agent. This is ~30 lines of identical boilerplate per command. Extract a shared `buildAgent()` helper.
- **Ingestion pipeline is a stub**: `ingest.go` prints a TODO message and returns nil. The `Pipeline` struct in `internal/ingestion/pipeline.go` is fully implemented but never wired. The `upsertWithEmbeddings` method ignores the embeddings parameter entirely — it calls `store.Upsert(docs)` without attaching vectors to the points.

---

## 3. Security

### What's Good

- **Path traversal protection**: `confineToDir()` uses separator-aware prefix checking in workspace, file, and generate handlers. This is the correct pattern.
- **Auth middleware**: Bearer token with `WWW-Authenticate` challenge. Token value never logged.
- **Rate limiting**: Per-IP token bucket with eviction. Prevents brute-force.
- **Body size cap**: `maxChatBodyBytes = 1 MiB` on `/api/chat`.
- **Non-root Docker user**: `USER tfai` in Dockerfile.
- **gosec enabled**: In golangci-lint config with targeted suppressions.

### Critical Gaps

1. **Token comparison is not constant-time** (`auth.go:41`):
   ```go
   if token != apiKey {
   ```
   This is vulnerable to timing attacks. Use `crypto/subtle.ConstantTimeCompare()`. **Severity: HIGH for any network-exposed deployment.**

2. **No TLS**: The server binds to `http://` only. Even on localhost, any process on the machine can sniff traffic including the Bearer token. The README says "local-first" but the Docker setup exposes port 8080 on `0.0.0.0`. **Severity: MEDIUM.**

3. **`/api/file` and `/api/workspace` expose the entire filesystem**: The only guard is `confineToDir()`, but the workspace root itself is user-supplied in the request body. An attacker with a valid API key can read/write any file the process user can access by setting `workspaceDir=/`. **Severity: HIGH.** Mitigation: enforce an allowlist of workspace roots, or require a server-side configured workspace base directory.

4. **`terraform plan` and `terraform state` execute arbitrary commands**: The `ExecRunner` runs `terraform <subcommand>` in a user-supplied directory. If the LLM is prompt-injected into calling `terraform_plan` with a malicious directory containing a poisoned `.tf` file with `local-exec` provisioners, it could execute arbitrary code. **Severity: HIGH for any multi-user deployment.** Mitigation: sandbox terraform execution, or at minimum log every tool invocation with full arguments.

5. **No CSRF protection**: The web UI makes state-changing requests (POST, PUT) from JavaScript. There's no CSRF token. The CORS check only validates `Origin` against localhost, which is bypassable. **Severity: LOW for local-only, MEDIUM if exposed.**

6. **`docker-compose.yml` has hardcoded secrets**:
   ```yaml
   NEXTAUTH_SECRET=change-me-in-production
   SALT=change-me-in-production
   POSTGRES_PASSWORD=langfuse
   ```
   These should be in `.env` or a secrets manager. **Severity: MEDIUM.**

7. **No input sanitization on chat messages**: User messages are passed directly to the LLM. While this is inherent to LLM applications, there's no prompt injection detection or guardrails. **Severity: informational — track as a known risk.**

### Recommendations for Audit Readiness

| # | Action | Priority | Effort |
|---|---|---|---|
| S1 | Replace `!=` with `crypto/subtle.ConstantTimeCompare` in `auth.go` | Critical | 1 line |
| S2 | Add `--workspace-root` flag to `serve` that confines all file ops | High | ~50 LOC |
| S3 | Log every tool invocation (plan/state) with full args before execution | High | ~10 LOC |
| S4 | Add TLS support (or document that a reverse proxy is required) | Medium | ~30 LOC |
| S5 | Move docker-compose secrets to `.env` | Medium | Config only |
| S6 | Document the threat model: what's trusted, what's not, what's in scope | Medium | Prose |

---

## 4. Testing

### Current State

| Package | Test Files | Coverage | Notes |
|---|---|---|---|
| `internal/server` | 7 files | Good | Auth, chat, file, health, metrics, ratelimit, workspace |
| `internal/agent` | 2 files | Partial | `apply_test.go`, `parse_test.go` — no test for `Query()` or `buildMessages()` |
| `internal/provider` | 1 file | Partial | `config_test.go` — validates `Validate()` only |
| `internal/store` | 1 file | Good | 5 tests covering CRUD, limits, isolation, ordering |
| `internal/budget` | 1 file | Good | 5 tests (one currently failing — fix in progress) |
| `internal/ingestion` | 0 | **None** | Pipeline is implemented but untested |
| `internal/rag` | 0 | **None** | Qdrant store, retriever — no unit tests |
| `internal/tools` | 0 | **None** | Plan, state, generate tools — no unit tests |
| `internal/logging` | 0 | **None** | Small package, low risk |
| `internal/tracing` | 0 | **None** | Thin wrapper, low risk |
| `internal/version` | 0 | **None** | Constants only, no logic |
| `cmd/` | 0 | **None** | CLI commands — no tests |

**Source lines:** 3,967 | **Test lines:** 2,106 | **Test ratio:** 0.53 (target: ≥1.0)

### Key Gaps

1. **No integration tests**: Qdrant, Ollama, and the full serve→agent→LLM path have zero automated tests. The `SMOKE_TEST.md` is manual.
2. **No test for `agent.Query()`**: The core business logic function — message building, streaming, history persistence — has no unit test. The `fakeQuerier` in server tests bypasses the agent entirely.
3. **No test for tool invocations**: `PlanTool`, `StateTool`, `GenerateTool` have no tests despite having real logic (input validation, path traversal checks, command construction).
4. **No fuzz testing**: `parseAgentOutput()` parses untrusted LLM output as JSON. A fuzz test would catch edge cases.

### Recommendations

| # | Action | Priority | Effort |
|---|---|---|---|
| T1 | Unit tests for `internal/tools` (fake Runner, validate args/paths) | High | ~200 LOC |
| T2 | Unit test for `agent.buildMessages()` with history + RAG + workspace | High | ~150 LOC |
| T3 | Integration test suite (`//go:build integration`) for Qdrant + Ollama | Medium | ~300 LOC |
| T4 | Fuzz test for `parseAgentOutput()` | Medium | ~30 LOC |
| T5 | Coverage gate in Makefile (`go test -coverprofile`, fail if < threshold) | Low | ~10 LOC |

---

## 5. Code Quality

### What's Good

- **Every exported symbol has a doc comment.** This is rare and valuable.
- **Every struct field has a comment.** Excellent for onboarding.
- **Consistent error wrapping**: `fmt.Errorf("package: context: %w", err)` pattern used everywhere.
- **golangci-lint config is thoughtful**: `gosec`, `wrapcheck`, `noctx`, `bodyclose` enabled with targeted suppressions.
- **No global mutable state** (except `requestCounter` atomic, which is fine).

### Issues

1. **`applyFiles` uses `0755` / `0644` without `0o` prefix** (`apply.go:24,30`): Inconsistent with the rest of the codebase which uses `0o644`. Not a bug (Go accepts both) but fails style consistency.

2. **`writeJSONError` builds JSON via string concatenation** (`workspace.go:33`):
   ```go
   http.Error(w, `{"error":"`+msg+`"}`, status)
   ```
   If `msg` contains a `"` character, this produces invalid JSON. Use `json.Marshal` or `fmt.Sprintf` with proper escaping.

3. **`ingest.go` has dead code** (`_ = ctx` on line 51): The context is captured but never used. The entire RunE body is a stub.

4. **`GenerateTool` exists but is never registered**: `buildTools()` in `helpers.go` explicitly excludes it (comment explains why), but the tool itself is 100 lines of maintained code that's unreachable. Either delete it or document why it's kept.

5. **Inconsistent error message style**: Some use `agent::applyFiles:` (double colon), others use `agent:` (single colon with space). Standardize on `package: function: message`.

6. **`Bedrock` backend uses `einoark` (Volcano Engine)**: The comment says "TODO: Replace with a dedicated Bedrock implementation." This is a correctness risk — the Ark provider is not a Bedrock client. It may work for some models but will fail for Bedrock-specific features.

---

## 6. Observability

### What's Good

- **Structured logging via `slog`**: JSON in production, text in dev. Context-propagated logger with `request_id`.
- **Langfuse tracing**: Opt-in, per-request session IDs, version-tagged.
- **Prometheus metrics**: Chat request counters, duration histograms, active stream gauge.

### Gaps

1. **`httpRequestsTotal` and `httpDurationSeconds` are registered but never incremented**: The metrics are defined in `metrics.go` but no middleware calls `WithLabelValues().Inc()` on them. They will always be zero.
2. **No `request_id` in agent logs**: The middleware generates a `request_id` and injects it into the request context, but `handleChat` creates a new logger with `session_id` instead of inheriting the request logger. The `request_id` is lost.
3. **No alerting rules or dashboards**: Metrics exist but there's no `alerts.yml` or Grafana dashboard JSON.

---

## 7. Operations & CI

### What's Good

- **Makefile with `gate` target**: Build → vet → lint → test → binary smoke. Enforced before every commit.
- **Multi-stage Dockerfile**: Builder + runtime, non-root user, pinned Terraform version.
- **docker-compose**: Full stack (app + Qdrant + Langfuse + Postgres).
- **Version injection via ldflags**: Clean `version.go` pattern.

### Gaps

1. **No CI pipeline**: No `.github/workflows/` CI file. The gate runs locally only. A PR could merge without passing any checks.
2. **No `govulncheck`**: CVE scanning is not part of the gate or any automated process.
3. **No release automation**: Tags are created manually. No goreleaser, no changelog generation.
4. **Dockerfile hardcodes `GOARCH=amd64`**: Won't build on ARM CI runners or Apple Silicon without cross-compilation.
5. **~~`.env` and `.env.azure` tracked by git~~** — CORRECTED: These files are gitignored and confirmed not in `git ls-files`. They exist locally only. Not a security incident.

---

## 8. RAG Pipeline

### Current State

The RAG pipeline is **architecturally complete but operationally broken**:

- `internal/rag/interface.go`: Clean interfaces (`VectorStore`, `Embedder`, `Retriever`).
- `internal/rag/qdrant.go`: Full Qdrant implementation (upsert, search, delete, ping, close).
- `internal/rag/retriever.go`: `DefaultRetriever` combines embedder + store.
- `internal/ingestion/pipeline.go`: Fetch → chunk → embed → upsert pipeline.

**But:**

1. **No `Embedder` implementation exists**: The `Embedder` interface is defined but there's no concrete implementation anywhere in the codebase. The ingestion pipeline requires one but can never be instantiated.
2. **`ingest` CLI command is a stub**: It prints a TODO and returns nil. The `Pipeline` is never constructed.
3. **`upsertWithEmbeddings` discards embeddings**: Line 145 has `_ = embeddings[i]` and the actual upsert sends points without vectors. This means even if the pipeline ran, Qdrant would store empty vectors.
4. **Chunking is character-based, not semantic**: The chunker splits on character count with overlap. For Terraform documentation (HCL blocks, markdown sections), this will split mid-resource-block, producing chunks that are semantically meaningless.
5. **No embedding model configuration**: Tracked in #34, blocked on architecture review #36.

### Verdict

The RAG pipeline should be considered **not functional**. The interfaces are good and the Qdrant client works, but the end-to-end path from `tfai ingest` to `agent.Query()` with RAG context has never been executed. This is tracked in #34, #35, #36.

---

## 9. Web UI

The UI is a single 1,048-line `index.html` file with inline CSS and JavaScript. It provides:

- Chat interface with SSE streaming
- Workspace file browser
- File editor with save
- Auth modal (prompts for API key when `auth_required=true`)

### Issues

- **No framework, no build step**: All JS is inline. This is fine for a prototype but will not scale.
- **No XSS protection on chat output**: LLM responses are rendered as HTML (markdown). If the LLM returns `<script>` tags, they could execute. Use a sanitizing markdown renderer.
- **API key stored in JavaScript variable**: `window._apiKey` is set from the modal and persisted in `sessionStorage`. This is visible to any browser extension or devtools.
- **Migration to Vite + React is tracked as #12** but deferred.

---

## 10. Roadmap — Prioritized

### Phase 1: Audit Readiness (before any external exposure)

| # | Item | Issue | Priority | Effort |
|---|---|---|---|---|
| 1.1 | Constant-time token comparison | New | Critical | 1 line |
| 1.2 | Verify `.env` files are not committed with real keys | New | Critical | Audit |
| 1.3 | Workspace root confinement (`--workspace-root` flag) | New | High | ~50 LOC |
| 1.4 | Log all tool invocations before execution | New | High | ~10 LOC |
| 1.5 | Fix `writeJSONError` JSON injection | New | High | ~5 LOC |
| 1.6 | Move docker-compose secrets to `.env` | New | Medium | Config |
| 1.7 | Document threat model | New | Medium | Prose |
| 1.8 | Add CI pipeline (GitHub Actions) | New | Medium | ~100 LOC |
| 1.9 | Add `govulncheck` to gate | qa1 | Medium | ~10 LOC |

### Phase 2: Test Coverage (before feature work resumes)

| # | Item | Issue | Priority | Effort |
|---|---|---|---|---|
| 2.1 | Unit tests for `internal/tools` | New | High | ~200 LOC |
| 2.2 | Unit test for `agent.buildMessages()` | New | High | ~150 LOC |
| 2.3 | Fix and land token budget (#22) | #22 | High | In progress |
| 2.4 | Integration test suite | qa2 | Medium | ~300 LOC |
| 2.5 | Fuzz test for `parseAgentOutput()` | New | Medium | ~30 LOC |

### Phase 3: RAG Pipeline (requires architecture decision first)

| # | Item | Issue | Priority | Effort |
|---|---|---|---|---|
| 3.1 | RAG architecture review (ADR) | #36 | High | Prose |
| 3.2 | Implement `Embedder` for Ollama + OpenAI | #34 | High | ~200 LOC |
| 3.3 | Wire `ingest` CLI command end-to-end | New | High | ~100 LOC |
| 3.4 | Fix `upsertWithEmbeddings` to attach vectors | New | High | ~20 LOC |
| 3.5 | Semantic chunking for HCL/Terraform docs | New | Medium | ~200 LOC |
| 3.6 | Reranking pipeline | #35 | Low | ~300 LOC |

### Phase 4: Production Hardening

| # | Item | Issue | Priority | Effort |
|---|---|---|---|---|
| 4.1 | Wire `httpRequestsTotal` / `httpDurationSeconds` metrics | New | Medium | ~30 LOC |
| 4.2 | TLS support or reverse proxy documentation | New | Medium | ~30 LOC |
| 4.3 | XSS protection in UI chat rendering | New | Medium | ~20 LOC |
| 4.4 | 3 Musketeers dev container | #10 | Medium | ~200 LOC |
| 4.5 | Release automation (goreleaser or equivalent) | New | Low | ~100 LOC |
| 4.6 | Multi-arch Dockerfile | New | Low | ~10 LOC |

### Phase 5: Feature Work

| # | Item | Issue | Priority |
|---|---|---|---|
| 5.1 | Token budget management | #22 | In progress |
| 5.2 | Hot-reload dev server | #11 | Low |
| 5.3 | Vite + React UI migration | #12 | Deferred |

---

## 11. What's Explicitly NOT a Problem

These are things I reviewed and found to be correct — documenting them to avoid false flags:

- **`confineToDir()` path traversal check**: Separator-aware prefix check is correct. The `filepath.Clean` + `HasPrefix` + separator append pattern handles `../`, symlinks-via-clean, and partial-match attacks.
- **Rate limiter eviction**: The 5-minute TTL with 1-minute sweep is appropriate for a local server.
- **SSE streaming**: The `sseWriter` correctly handles multi-line chunks with per-line `data:` prefixes.
- **Graceful shutdown**: Signal handling → context cancellation → `http.Server.Shutdown` → rate limiter stop. Correct order.
- **Provider validation**: `Config.Validate()` checks all required fields per backend at startup, not at request time. This is the right pattern.

---

## 12. Codebase Statistics

| Metric | Value |
|---|---|
| Go source lines (non-test) | 3,967 |
| Go test lines | 2,106 |
| Test-to-source ratio | 0.53 |
| Packages with tests | 5 / 12 (42%) |
| Packages without tests | 7 (ingestion, rag, tools, logging, tracing, version, cmd) |
| UI lines (HTML/CSS/JS) | 1,048 |
| External dependencies | ~40 (go.mod) |
| Docker images | 4 (app, qdrant, langfuse, postgres) |
| Git tags | v0.13.0 → v0.17.0 (5 releases) |
| Open issues | #10, #11, #12, #20–22, #34–36 |
