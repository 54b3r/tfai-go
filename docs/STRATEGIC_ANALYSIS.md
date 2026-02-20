# TF-AI-Go — Strategic Analysis: Accelerator vs Product

**Date:** 2026-02-20  
**Context:** Should tfai-go be refactored into a reusable "accelerator" framework for building AI-powered platform engineering tools?

---

## Part 1: REVIEW.md Corrections & Additions

### Corrections

- **`.env` files are NOT tracked in git.** They are gitignored and confirmed not in `git ls-files`. The REVIEW.md flagged this as a potential security incident — it is not. Corrected.

### Items Missed in REVIEW.md

1. **Shutdown timeout vs chat timeout mismatch**: `ShutdownTimeout` defaults to 10 seconds but `ChatTimeout` defaults to 5 minutes. If a chat stream is mid-flight when SIGTERM arrives, `http.Server.Shutdown` will wait 10 seconds then kill the connection. The user gets a broken SSE stream with no `event: done` marker. The UI has no reconnection logic. This is a data-loss path for conversation history (the assistant message won't be persisted because `Query()` hasn't returned yet).

2. **`requestCounter` is process-scoped**: Session IDs are `tfai-{timestamp}-{counter}`. In a multi-replica k8s deployment, two pods could generate identical session IDs at the same millisecond. This breaks Langfuse trace deduplication. Should include hostname or pod name.

3. **UI static files resolved relative to CWD**: `filepath.Abs("ui/static")` depends on the working directory when the binary starts. In Docker this works (WORKDIR /app). In a k8s pod with a different CWD, or when running `./bin/tfai serve` from a parent directory, the UI silently serves 404s with no error log.

4. **No body size limit on non-chat endpoints**: `/api/chat` has `maxChatBodyBytes = 1 MiB`, but `/api/workspace/create` and `/api/file` (PUT) have no body size limits. A malicious request could POST a multi-GB JSON body.

5. **SQLite store has no Ping/health method**: The conversation store has no way to report its health to the readiness probe. If the SQLite file gets corrupted or the disk fills, the readiness endpoint won't reflect it.

6. **No pagination on workspace file listing**: `handleWorkspace` walks the entire directory tree synchronously. A large Terraform monorepo (thousands of files, nested modules) could produce a response measured in megabytes and take seconds to walk.

7. **`LLMPinger.Ping()` sends a full generate request**: The readiness probe sends `"ping"` to the LLM and waits for a complete response. This consumes tokens on paid APIs (OpenAI, Azure, Gemini) on every readiness check cycle. For k8s with a 10-second probe interval, that's 8,640 LLM calls/day just for health checks.

8. **`callbacks.AppendGlobalHandlers(handler)` mutates global state**: This is called in `serve.go` and affects ALL Eino operations process-wide. In tests or if multiple agents were ever constructed, this leaks tracing state across boundaries.

9. **No SBOM (Software Bill of Materials)**: For enterprise/audit, you need to produce an SBOM artifact. `govulncheck` tells you about known vulns; an SBOM tells auditors what's in the box.

10. **Qdrant port hardcoded to 6334 in `helpers.go`**: `buildPingers` creates a Qdrant client with `Port: 6334` ignoring `QDRANT_PORT` from the environment. The docker-compose and `.env.example` both reference this var, but the readiness probe won't respect it.

---

## Part 2: The Accelerator Question — Honest Assessment

### What You've Actually Built

Let me decompose tfai-go into its layers:

| Layer | What It Does | Lines | Reusable? |
|---|---|---|---|
| **LLM Provider Factory** | Multi-backend LLM construction from env vars | ~400 | Eino already does this |
| **HTTP Server Shell** | Mux + auth + rate-limit + SSE streaming + health probes | ~900 | Yes, but generic |
| **Agent Core** | ReAct loop + message building + tool dispatch | ~350 | Eino already does this |
| **Domain Tools** | Terraform plan/state/generate | ~380 | Terraform-specific |
| **Domain Prompt** | System prompt, workspace context, file generation | ~200 | Terraform-specific |
| **RAG Pipeline** | Interfaces + Qdrant + ingestion (broken) | ~560 | Yes, but incomplete |
| **Conversation Store** | SQLite history | ~160 | Yes |
| **Observability** | Logging + Langfuse + Prometheus metrics | ~200 | Yes, but thin wrappers |
| **CLI Skeleton** | Cobra commands + version | ~350 | Boilerplate |
| **Web UI** | Single-page chat + file browser | ~1050 | Partially reusable |

**Of ~4,500 lines of Go + 1,050 lines of UI:**
- ~1,100 lines (24%) are **domain-specific** (Terraform tools, prompts, file handling)
- ~1,300 lines (29%) are **generic server infra** (auth, rate-limit, health, metrics, SSE)
- ~960 lines (21%) duplicate what **Eino already provides** (provider factory, agent core)
- ~720 lines (16%) are **potentially reusable utilities** (store, budget, logging wrappers)
- ~420 lines (9%) are **CLI boilerplate** (Cobra scaffolding)

### The Core Question: Is Extracting a Framework Worth Your Time?

**No. Not now. Here's why:**

#### 1. You're building on top of a framework that already exists

Eino (`cloudwego/eino`) already provides:
- Chat model abstraction with multi-provider support
- ReAct agent with tool-calling
- Streaming responses
- Callback/tracing hooks

What tfai-go adds on top of Eino is an HTTP server, auth middleware, and domain-specific tools. That's an **application**, not a framework. Extracting the server shell into a reusable library gives you... an HTTP server with auth and rate limiting. That's not novel — it's what every Go service needs, and there are dozens of existing solutions (go-chi, echo, fiber + middleware).

#### 2. The Rule of Three

You have exactly **one** domain implementation (Terraform), and it's not finished (RAG is broken, ingestion is a stub). You don't yet know what generalizes because you haven't built a second thing. Framework extraction before building 2-3 concrete products leads to:
- Wrong abstractions (you'll guess at what's shared and get it wrong)
- Over-engineering (making things configurable that should be hardcoded)
- Slower iteration (every change requires updating the framework + the consumer)

#### 3. The value is in domain expertise, not infrastructure

For a Terraform AI assistant, the hard parts are:
- **System prompt engineering** that produces correct HCL
- **Tool definitions** that match Terraform's actual workflow
- **RAG pipeline** that understands HCL semantic boundaries
- **Understanding what platform engineers actually struggle with**

None of these generalize to a framework. They're the product.

#### 4. The market for "AI app frameworks in Go" is crowded and losing

Python dominates the LLM tooling ecosystem (LangChain, LlamaIndex, CrewAI, Haystack, Semantic Kernel). Go is a fine choice for the final compiled binary, but building a Go framework to compete with the Python ecosystem's breadth is a losing position for a single engineer.

### What WOULD Be Worth Your Time

Here are three strategic options, ranked by engineering ROI:

---

### Option A: Finish the Product (Recommended)

**Thesis:** Ship tfai-go as a working Terraform AI assistant that platform engineers actually use. Iterate based on real usage. Don't extract anything.

**Why this wins:**
- You get to "does anyone actually want this?" fastest
- Every hour goes into the product, not meta-infrastructure
- If it works for Terraform, you can clone the repo and adapt it for K8s/CI/CD later — copy-paste is fine for product #2

**What "done" looks like:**
1. Fix the security gaps (Phase 1 of REVIEW.md roadmap — 1-2 days)
2. Land token budget (#22 — hours, already in progress)
3. RAG architecture decision + embedder implementation (#34, #36 — 3-5 days)
4. End-to-end `tfai ingest` → `tfai serve` with real RAG — the first time a user can actually benefit from indexed docs
5. CI pipeline (GitHub Actions — 1 day)
6. A real user runs it against a real Terraform repo and gives feedback

**Timeline:** 2-3 weeks of focused work to a usable v1.0.

---

### Option B: Template Repo, Not Framework

**Thesis:** Instead of extracting a shared library, create a **project template** (cookiecutter/scaffold) that stamps out a new AI-powered CLI+server with the same patterns.

**What the template includes:**
- Makefile with gate, docker-compose, Dockerfile
- `cmd/` + `internal/` structure with Cobra
- Server shell with auth, rate-limit, health probes, metrics, SSE
- Provider factory (Eino-based, multi-backend)
- Conversation store (SQLite)
- `.golangci.yml`, `.github/workflows/ci.yml`
- Empty `internal/tools/` with a sample tool interface
- Empty `internal/agent/` with a sample system prompt
- README template

**Why this is better than a framework:**
- No shared dependency to version and maintain
- Each project can diverge without breaking others
- New project in 5 minutes: `cookiecutter gh:54b3r/platform-ai-template`
- The "framework" is the pattern, not the code

**Effort:** ~2 days after tfai-go is stable. You're literally packaging what you already have.

---

### Option C: MCP Server (The Pivot Worth Considering)

**Thesis:** Instead of a standalone CLI+server, build tfai as an **MCP (Model Context Protocol) server** that integrates into existing AI IDEs (Claude Code, Copilot, Cursor, Windsurf).

**Why this might be more impactful:**

The standalone UI you built (`index.html`) is competing with every AI chat interface in existence. Platform engineers already live in their IDE or terminal. An MCP server lets them use tfai's Terraform expertise from within the tools they already use — no new UI to learn, no new server to run.

**What an MCP server provides:**
- **Tools**: `terraform_plan`, `terraform_state`, `terraform_generate` — exposed as MCP tools that Claude/Copilot can call
- **Resources**: Workspace `.tf` files, state, plan output — exposed as MCP resources
- **Prompts**: Domain-specific system prompts for Terraform tasks

**What you'd keep from tfai-go:**
- `internal/tools/` (plan, state, generate) → MCP tool handlers
- `internal/rag/` → MCP resource provider for indexed docs
- `internal/store/` → conversation persistence (optional, the IDE may handle this)
- Provider factory → NOT needed (the IDE provides the LLM)

**What you'd drop:**
- `internal/server/` (the whole HTTP server, auth, rate-limit, SSE — the IDE handles all of this)
- `internal/agent/` (the ReAct loop — the IDE's AI handles tool-calling)
- `internal/provider/` (the IDE provides the LLM)
- `ui/` (the IDE IS the UI)
- ~60% of the codebase

**Effort:** ~1 week to build a basic MCP server exposing plan/state/generate tools. The Go MCP SDK exists (`github.com/mark3labs/mcp-go`).

**The honest risk:** MCP is still early. Not all IDEs support it equally. You'd be betting on the ecosystem maturing. But the trend is clear — AI tooling is converging on MCP as the protocol for tool integration.

---

## Part 3: The Enterprise/K8s Deployment Question

You mentioned wanting to deploy solutions to enterprise infrastructure like k8s. Here's what that actually requires vs what you have:

| Requirement | Current State | Gap |
|---|---|---|
| **Container image** | Multi-stage Dockerfile, non-root user | ✅ Done |
| **Health probes** | `/api/health` (liveness), `/api/ready` (readiness) | ✅ Done |
| **Graceful shutdown** | Signal handling + drain | ⚠️ Timeout mismatch (see above) |
| **Structured logging** | slog JSON to stderr | ✅ Done |
| **Metrics** | Prometheus `/metrics` | ⚠️ HTTP metrics not wired |
| **Config via env vars** | All config is env-based | ✅ Done |
| **Secrets management** | Reads from env (k8s Secret → env) | ✅ Done |
| **Horizontal scaling** | Session IDs collide across replicas | ❌ Fix needed |
| **Helm chart** | None | ❌ Needed |
| **Network policy** | None | ❌ Needed for enterprise |
| **RBAC / multi-tenancy** | Single Bearer token, no user identity | ❌ Major gap |
| **Audit logging** | Request logging, no audit trail | ❌ Needed for compliance |
| **SBOM** | None | ❌ Needed for enterprise |
| **SOC2/ISO controls** | No evidence artifacts | ❌ Needed for enterprise |

**Honest take:** You're ~70% of the way to "deployable on k8s" but ~30% of the way to "enterprise-ready." The gaps are mostly organizational/compliance (Helm, RBAC, audit, SBOM), not architectural.

---

## Part 4: My Recommendation

**Go with Option A (Finish the Product) now, keep Option B (Template Repo) in mind for later.**

Here's the reasoning:

1. **Your time is finite.** Framework engineering feels productive but delays the moment someone uses your tool and tells you if it's valuable.

2. **tfai-go's architecture is already good enough.** The package boundaries are clean, the interfaces are right, the middleware chain is correct. You don't need to refactor to build a second product — you need to finish the first one.

3. **The "accelerator" is the patterns in your head.** After finishing tfai-go, building a k8s troubleshooter or CI/CD advisor will take you 1/3 the time because you've already solved the auth/metrics/health/store/streaming problems. You don't need shared code for that — you need the experience of having done it once.

4. **If you want maximum impact with minimum engineering time**, seriously evaluate Option C (MCP server). It cuts 60% of the codebase and puts your Terraform expertise directly into the tools platform engineers already use. The standalone server+UI path is a much harder go-to-market.

5. **Don't build a plugin system.** Plugin architectures are the #1 trap for engineers who want to build "extensible" things. They're expensive to design, expensive to maintain, and the second plugin always invalidates the abstraction you built for the first one. Build concrete tools, not plugin interfaces.

### Suggested Next Steps (in order)

1. **Land #22** (token budget) — it's 90% done, just needs `make gate`
2. **Fix the critical security items** from REVIEW.md Phase 1 (S1: constant-time compare, S3: tool logging) — 30 minutes
3. **Spike on MCP** — spend 2 hours exploring `mcp-go`, see if exposing `terraform_plan` as an MCP tool works in Claude/Cursor. If it does, that's your next branch.
4. **RAG architecture decision** (#36) — this blocks everything else in the product
5. **Ship a v1.0 that a real user can test against a real Terraform repo**

---

## Part 5: What I Would NOT Spend Time On Right Now

| Item | Why Not |
|---|---|
| Vite + React UI migration (#12) | If MCP is the right path, the UI is irrelevant. If not, the current UI works for demos. |
| 3 Musketeers dev container (#10) | Nice-to-have, not blocking anything. |
| Reranking pipeline (#35) | Premature — you don't have basic RAG working yet. |
| Hot-reload dev server (#11) | Quality of life, not product value. |
| Helm chart | Only needed when you have a user who needs it. |
| Plugin architecture | See above — don't build it. |
