# TF-AI-Go — Roadmap & Project Tracker

> **Single source of truth.** Every shipped feature, open gap, and planned item lives here.
> Updated after every merge to `main`. Cross-referenced with GitHub Issues.

**Last updated:** 2026-02-24
**Current version:** v0.29.0
**Branch policy:** All work on feature branches → PR → merge → tag → update this file.  
**Sources:** [REVIEW.md](./REVIEW.md) · [SRE_ASSESSMENT.md](./SRE_ASSESSMENT.md) · [STRATEGIC_ANALYSIS.md](./STRATEGIC_ANALYSIS.md)

---

## 1. Release History — What's Shipped

| Version | Date | Summary | Key PRs |
|---|---|---|---|
| **v0.29.0** | 2026-02-24 | Generate model override — separate LLM for code generation via `GENERATE_*` env vars, golangci-lint v2 migration | #58 |
| **v0.28.0** | 2026-02-23 | Security hardening — per-IP rate limiting, request ID header, audit log, structured startup/shutdown logging | #57 |
| **v0.25.0** | 2026-02-22 | Backstage integration — catalog entity, scaffolder template, YAML-first config shift | #49 |
| **v0.24.0** | 2026-02-22 | YAML config file support (`internal/config`), structured CLI audit logging (`internal/audit`) | #48 |
| **v0.23.0** | 2026-02-22 | RAG metadata auto-inference from URLs, expanded Makefile ingest targets, structured Qdrant payload | #45 |
| **v0.22.0** | 2026-02-21 | Embedder config guardrails — fail-fast validation, QDRANT_PORT fix, nil-guard qdrant client | #43, #44 |
| **v0.21.0** | 2026-02-21 | RAG pipeline wired end-to-end — ingest → embed → store → retrieve → serve | #43 |
| **v0.20.0** | 2026-02-21 | RAG foundation — embedder factory, VectorStore.Upsert fix, zero-cost LLM health checks | #43 |
| **v0.19.0–v0.19.1** | 2026-02-20 | Azure reasoning model fix, system prompt v2, security hardening (constant-time auth, tool audit logging), govulncheck, SRE assessment | #39, #41, #42 |
| **v0.18.0** | 2026-02-20 | Token budget management, codebase review, strategic analysis | #38 |
| **v0.17.0** | 2026-02-20 | Conversation history (SQLite) | #37 |
| **v0.16.0 and earlier** | Pre-2026-02-20 | Core agent, serve/ask/generate commands, web UI, Terraform tools, Prometheus metrics, auth, rate limiting, health probes, structured logging, graceful shutdown | — |

---

## 2. Closed GitHub Issues

| # | Title | Closed |
|---|---|---|
| #47 | Backstage integration — catalog entry, scaffolder, deployment guide | 2026-02-22 |
| #34 | RAG: dedicated embedding model selection | 2026-02-22 |
| #30 | Wire Authorization header in web UI fetch calls | 2026-02-20 |
| #29 | Wire context deadline on LLM calls in handleChat | 2026-02-20 |
| #22 | Token budget management | 2026-02-20 |
| #21 | Conversation history persistence | 2026-02-20 |
| #20 | Prometheus metrics endpoint | 2026-02-20 |
| #19 | Authentication middleware | 2026-02-20 |
| #18 | HTTP handler test coverage | 2026-02-20 |
| #17 | Rate limiting | 2026-02-20 |
| #16 | Deep health/readiness probes | 2026-02-20 |
| #15 | Config validation at startup | 2026-02-20 |
| #14 | Structured logging with slog + request IDs | 2026-02-20 |
| #13 | Graceful shutdown with in-flight draining | 2026-02-20 |

---

## 3. Open GitHub Issues

| # | Title | Category | Roadmap Tier | Priority |
|---|---|---|---|---|
| **#46** | LLM-based metadata classification (`--classify` flag) | RAG | Tier 3 | Low |
| **#40** | YAML config — hot-reload + multi-model support | Config | Tier 2 | Medium |
| **#36** | RAG architecture review — naive vs advanced patterns | RAG | Tier 2 | Medium |
| **#35** | RAG reranking pipeline — cross-encoder / RRF | RAG | Tier 3 | Low |
| **#12** | Migrate UI to Vite + React | UI | Tier 3 | Low (blocked by MCP decision) |
| **#11** | `.air.toml` + `make dev` for hot-reload | DX | Tier 2 | Medium |
| **#10** | 3 Musketeers dev container | DX | Tier 3 | Low |

---

## 4. Full Audit — Current State (2026-02-22)

> Audit performed from **Platform Engineering, SRE, Security, and Go code quality** perspectives.
> Each item is verified against the actual codebase, not prior assumptions.

### 4.1 What's Working Well

| Area | Detail |
|---|---|
| **Auth** | Bearer token with `crypto/subtle.ConstantTimeCompare`. Warn on disabled. |
| **Rate limiting** | Token bucket on protected routes. Configurable rate/burst. |
| **Graceful shutdown** | `signal.NotifyContext` + `httpServer.Shutdown` with configurable timeout. |
| **Health/Readiness** | `/api/health` (liveness) + `/api/ready` (per-dependency probes with timeout). |
| **Structured logging** | `log/slog` throughout. Request ID injected by middleware, propagated via context. |
| **Chat timeout** | `context.WithTimeout` on LLM calls. Default 5 min. |
| **Body limit on chat** | `http.MaxBytesReader` 1 MiB on `/api/chat`. |
| **Path traversal guard** | `confineToDir()` validates workspace paths. |
| **Prometheus metrics** | Registered: `tfai_chat_requests_total`, `tfai_chat_duration_seconds`, `tfai_chat_active_streams`. `/metrics` endpoint exposed. |
| **CI** | `.github/workflows/ci.yml` — build, vet, test, lint on push + PR. |
| **Release** | `.github/workflows/release.yml` — multi-arch binaries + checksums + GitHub Release on tag. |
| **Docker** | Multi-stage build, non-root user, pinned terraform binary. |
| **RAG pipeline** | End-to-end: ingest → embed → Qdrant → retrieve → inject into LLM context. |
| **YAML config** | Layered config: defaults → YAML → env vars. Search chain: `--config` → `$TFAI_CONFIG` → `~/.tfai/config.yaml` → `./tfai.yaml`. |
| **Audit logging** | CLI audit log on startup with secret key sanitisation. |
| **Backstage** | `catalog-info.yaml` (Component + API + Resource), scaffolder template, deployment guide. |
| **Conversation history** | SQLite-backed, auto-trimmed by token budget. |

### 4.2 Gaps Found — Verified Against Code

#### 4.2.1 SRE / Observability

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **MF-1** | `httpRequestsTotal` and `httpDurationSeconds` registered in `metrics.go` but **never incremented** — no `metricsMiddleware` exists | **High** | `server/metrics.go`, `server/middleware.go` | ❌ Not done |
| **LOG-2** | `handleChat` logs `"chat start"` but no `"chat complete"` success log | Medium | `server/server.go` | ❌ Not done |
| **SF-1** | No pprof debug endpoint | Low | — | ❌ Not done |
| **OBS-1** | No business metrics (tool invocations, RAG docs injected, history trimmed) | Medium | — | ❌ Not done |
| **OBS-3** | No Langfuse trace ID in structured logs | Low | — | ❌ Not done |
| **CI-2** | CI missing `govulncheck` (local `make gate` has it, CI doesn't) | Medium | `.github/workflows/ci.yml` | ❌ Not done |
| **CI-3** | CI missing binary smoke test | Low | `.github/workflows/ci.yml` | ❌ Not done |
| **CI-4** | No container image build/push in CI | Low | — | ❌ Not done |

#### 4.2.2 Security

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **MF-2** | No `MaxBytesReader` on `/api/workspace/create` or `/api/file` PUT — OOM risk | **High** | `server/workspace.go` | ❌ Not done |
| **SEC-2** | No `--workspace-root` flag to confine file operations to a directory | **High** | `server/workspace.go` | ❌ Not done |
| **SEC-5** | `writeJSONError` interpolates `msg` directly into JSON string — injection risk | Medium | `server/workspace.go:33` | ❌ Not done |
| **SEC-6** | `docker-compose.yml` has hardcoded secrets (NEXTAUTH_SECRET, SALT, POSTGRES_PASSWORD) | Medium | `docker-compose.yml` | ❌ Not done |
| **SEC-4** | No `terraform state` output redaction for sensitive values | Low | `tools/state.go` | ❌ Not done |
| **SEC-7** | No LICENSE file in repository | Medium | — | ❌ Not done |

#### 4.2.3 Resilience / Resource Management

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **MF-3** | `buildWorkspaceContext` reads ALL `.tf` files with no file count, per-file size, or total size cap — OOM risk on monorepos | **High** | `agent/agent.go` | ❌ Not done |
| **NH-2** | `agent.Query()` accumulates entire LLM response in `msgBuf` with no size cap | Medium | `agent/agent.go` | ❌ Not done |
| **SF-2** | `ExecRunner.Run` has no dedicated timeout — relies solely on caller context | Medium | `tools/runner.go` | ❌ Not done |
| **NH-1** | No max concurrent chat streams semaphore | Medium | `server/server.go` | ❌ Not done |
| **SF-6** | No `event: error` sent to active SSE streams on SIGTERM | Low | `server/server.go` | ❌ Not done |
| **K8S-4** | Dockerfile hardcodes `GOARCH=amd64` — no multi-arch build | Low | `Dockerfile` | ❌ Not done |

#### 4.2.4 Config System

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **CFG-7** | `config.Load()` uses `os.Setenv` as bridge — global side effect, makes testing harder | Low | `config/config.go` | Technical debt |
| **CFG-8** | No config validation at startup (missing provider, invalid port, etc.) | Medium | `config/config.go` | ❌ Not done |
| **CFG-4** | No hot-reload (#40) | Low | — | ❌ Not done |
| **CFG-9** | `serve` command still reads all config from env vars directly (not from unified config struct) | Low | `commands/serve.go`, `commands/helpers.go` | Technical debt |
| **CFG-10** | No env var interpolation (`${VAR}`) in YAML values | Low | `config/config.go` | ❌ Not done |

#### 4.2.5 Test Coverage

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **TEST-1** | Zero tests for `internal/tools` (plan, state, generate, runner) | **High** | `tools/*.go` | ❌ Not done |
| **TEST-5** | Zero tests for `internal/rag` (qdrant store, retriever) | **High** | `rag/*.go` | ❌ Not done |
| **TEST-6** | Zero tests for `internal/tracing` | Low | `tracing/*.go` | ❌ Not done |
| **TEST-7** | Zero tests for `internal/logging` | Low | `logging/*.go` | ❌ Not done |
| **TEST-8** | Zero tests for `cmd/tfai/commands` (CLI wiring) | Medium | `commands/*.go` | ❌ Not done |
| **TEST-3** | No fuzz tests for `parseAgentOutput()` | Low | `agent/parse_test.go` | ❌ Not done |
| **TEST-4** | No integration test suite (`//go:build integration`) | Medium | — | ❌ Not done |

#### 4.2.6 Developer Experience / Open Source

| ID | Finding | Severity | File(s) | Status |
|---|---|---|---|---|
| **DX-5** | No CONTRIBUTING.md | Medium | — | ❌ Not done |
| **DX-6** | No LICENSE file | **High** | — | ❌ Not done |
| **DX-7** | `docker-compose.yml` uses deprecated `version: "3.9"` key | Low | `docker-compose.yml` | ❌ Not done |

---

## 5. Prioritised Work Items

### Priority 1 — Critical (do next, blocks trust/safety)

These items represent **security vulnerabilities, data loss risks, or broken observability** that should be fixed before any feature work.

| ID | Item | Issue | Effort | Impact |
|---|---|---|---|---|
| **MF-2** | Body size limits on `/api/workspace/create` + `/api/file` PUT | #50 | ~5 LOC | Prevents OOM crash |
| **MF-3** | Cap `buildWorkspaceContext` (max files, max file size, max total) | #50 | ~20 LOC | Prevents OOM on monorepos |
| **MF-1** | Wire dead `httpRequestsTotal` + `httpDurationSeconds` metrics | #50 | ~30 LOC | Metrics actually work |
| **SEC-5** | Fix `writeJSONError` JSON injection | #50 | ~5 LOC | Prevents response injection |
| **SEC-7** / **DX-6** | Add LICENSE file | #51 | 1 file | Legal — blocks any open-source use |
| **LOG-2** | Add `"chat complete"` success log | #50 | ~5 LOC | Can confirm successful responses |

**Estimated total: ~65 LOC + 1 file. One branch, one PR, ~2 hours.**

### Priority 2 — High (this week)

| ID | Item | Issue | Effort |
|---|---|---|---|
| **SEC-2** | `--workspace-root` flag to confine file operations | #52 | ~50 LOC |
| **CI-2** | Add `govulncheck` to CI workflow | — (create) | ~10 LOC |
| **TEST-1** | Unit tests for `internal/tools` | #53 | ~200 LOC |
| **TEST-5** | Unit tests for `internal/rag` | #53 | ~150 LOC |
| **SEC-6** | Move docker-compose hardcoded secrets to `.env` reference | — | Config change |
| **CFG-8** | Config validation at startup (required fields, valid ranges) | #40 | ~40 LOC |
| **NH-2** | Cap response buffer in `agent.Query()` | #50 | ~10 LOC |

### Priority 3 — Medium (this sprint / 2 weeks)

| ID | Item | Issue | Effort |
|---|---|---|---|
| **SF-2** | Terraform command execution timeout (2 min default) | — (create) | ~10 LOC |
| **NH-1** | Max concurrent chat streams semaphore | — (create) | ~15 LOC |
| **OBS-1** | Business metrics (tool invocations, RAG docs, history trim) | — (create) | ~35 LOC |
| **DX-2** | Hot-reload dev server (`.air.toml` + `make dev`) | #11 | ~30 LOC |
| **DX-5** | CONTRIBUTING.md | — (create) | Prose |
| **CFG-10** | `${ENV_VAR}` interpolation in YAML config values | #40 | ~30 LOC |
| **CFG-9** | Refactor `serve` to read from unified config struct, not env directly | #40 | ~80 LOC |

### Priority 4 — Backlog (future sprints)

| ID | Item | Issue | Effort |
|---|---|---|---|
| **CFG-4** | Config hot-reload with fsnotify | #40 | ~90 LOC |
| **MM-1–4** | Multi-model support (chat / code / embedding) — **code gen override shipped v0.29.0**, embedding override TODO | #40 | ~50 LOC remaining |
| **MCP-1** | MCP server spike (2-hour timeboxed) | — (create) | ~100 LOC |
| **RAG-1** | RAG architecture ADR | #36 | Prose |
| **RAG-6** | Reranking pipeline | #35 | ~300 LOC |
| **#46** | LLM-based metadata classification | #46 | ~200 LOC |
| **SF-1** | pprof debug endpoint | — | ~15 LOC |
| **SF-6** | SSE error event on SIGTERM | — | ~20 LOC |
| **K8S-1** | Helm chart | — | ~300 LOC |
| **K8S-4** | Multi-arch Dockerfile | — | ~10 LOC |
| **UI-1** | Vite + React migration (only if MCP is NOT primary) | #12 | ~2000 LOC |
| **DX-1** | 3 Musketeers dev container | #10 | ~200 LOC |
| **REL-1** | goreleaser automation | — | ~100 LOC |
| **REL-3** | Container image push to GHCR | — | ~30 LOC |
| **RBAC-1** | Multi-user auth (JWT/OIDC) | — | ~200 LOC |
| **TEST-4** | Integration test suite | — | ~300 LOC |
| **DASH-1** | Grafana dashboard JSON | — | ~200 LOC |

---

## 6. GitHub Issue Tracker — Gap Analysis

Items that exist in this roadmap but have **no GitHub issue yet**:

### Create immediately (Priority 1–2 items)
- [x] Hardening: dead metrics (MF-1), body limits (MF-2), workspace caps (MF-3), JSON injection (SEC-5), chat complete log (LOG-2) → **#50**
- [x] LICENSE file → **#51**
- [x] `--workspace-root` confinement (SEC-2) → **#52**
- [ ] CI: add govulncheck (CI-2)
- [x] Unit tests: `internal/tools` (TEST-1), `internal/rag` (TEST-5) → **#53**
- [x] Response buffer cap (NH-2) → included in **#50**
- [ ] Config validation at startup (CFG-8) — tracked under **#40**

### Create when starting (Priority 3–4 items)
- [ ] Terraform command timeout (SF-2)
- [ ] Max concurrent streams (NH-1)
- [ ] Business metrics (OBS-1)
- [ ] CONTRIBUTING.md (DX-5)
- [ ] Config env var interpolation (CFG-10)
- [ ] Config struct refactor (CFG-9)
- [ ] MCP server spike (MCP-1)
- [ ] Helm chart (K8S-1)
- [ ] Multi-arch Dockerfile (K8S-4)
- [ ] goreleaser (REL-1)
- [ ] Container image CI (REL-3)
- [ ] Multi-user auth (RBAC-1)
- [ ] Integration tests (TEST-4)
- [ ] Grafana dashboard (DASH-1)

---

## 7. Decision Log

| Date | Decision | Rationale |
|---|---|---|
| 2026-02-22 | YAML-first configuration across repo | Cloud-native standard. `config.yaml` for settings, `.env` for secrets only. |
| 2026-02-22 | Backstage integration as catalog + scaffolder only | No runtime dependency on Backstage. Self-service provisioning. |
| 2026-02-20 | Do NOT build accelerator/framework | Rule of Three — only 1 domain impl. Eino provides agent framework. |
| 2026-02-20 | Do NOT build plugin architecture | Over-engineering. Build concrete tools. |
| 2026-02-20 | Defer UI migration (#12) | MCP spike may eliminate need for custom UI. |
| 2026-02-20 | Defer reranking (#35) | Basic RAG works. Needs architecture review first. |
| 2026-02-20 | Finish product (Option A) | Ship v1.0, get feedback, then decide template repo vs MCP. |
| 2026-02-20 | YAML config before multi-model | Env vars don't scale. YAML aligns with k8s ConfigMap. |

---

## 8. How to Release

### Versioning Convention

| Change Type | Version Bump | Example |
|---|---|---|
| Breaking changes | Major (when v1+) | v1.0.0 → v2.0.0 |
| New features | Minor | v0.29.0 → v0.30.0 |
| Bug fixes only | Patch | v0.29.0 → v0.29.1 |
| Pre-release testing | RC suffix | v0.30.0-rc.1 |

### Release Checklist

```bash
# 1. Ensure you're on main with all changes merged
git checkout main && git pull

# 2. Run the full gate locally — must pass
make gate

# 3. Update this file (docs/ROADMAP.md)
#    - Add entry to Release History table
#    - Update "Current version" at top
#    - Mark completed items in audit sections

# 4. Commit the release prep
git add docs/ROADMAP.md
git commit -m "chore: prepare vX.Y.Z release"
git push

# 5. Create and push the tag
git tag vX.Y.Z
git push --tags

# 6. Verify the release
#    - Check GitHub Actions: release workflow should trigger
#    - Check GitHub Releases: binaries and checksums attached
#    - Download and test a binary on your platform
```

### Tag Format

Tags **must** follow semantic versioning: `vMAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH-rc.N`

- ✅ `v0.30.0`, `v1.0.0`, `v0.30.0-rc.1`
- ❌ `v0.30`, `0.30.0`, `release-0.30.0`

### What the Release Workflow Does

1. Validates the tag format
2. Runs full test suite (build, vet, lint, test)
3. Builds binaries for: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
4. Generates SHA256 checksums
5. Creates GitHub Release with auto-generated notes from merged PRs

---

## 9. How to Update This File

1. After every merge to `main`: update the **Release History** table and mark completed items.
2. After creating a GitHub issue: add the issue number to the relevant row.
3. After a decision: add a row to the **Decision Log**.
4. Run `git diff docs/ROADMAP.md` before committing to verify changes are intentional.
