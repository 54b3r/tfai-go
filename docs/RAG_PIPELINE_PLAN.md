# RAG Pipeline Plan — `feat/rag-pipeline` PR

> Temporary execution plan for this PR cycle. Delete after merge.

## Current State

The RAG infrastructure exists structurally but is non-functional:

1. **`ingest.go`** — stub: prints TODO, never connects to Qdrant or an embedder
2. **`qdrant.go` Upsert** — creates points **without vectors** (embeddings computed
   then silently dropped) — search would always return empty results
3. **`pipeline.go`** — `upsertWithEmbeddings` was a bridge method that discarded embeddings
4. **`pingers.go` LLMPinger** — sends `Generate("ping")` to the LLM on every
   `/api/ready` call, burning tokens for a health check that should be zero-cost
5. **No Embedder implementation** — `rag.Embedder` interface exists but has no concrete impl
6. **Ingest sources** — only 3 AWS URLs, 2 Azure, 2 GCP — insufficient for meaningful RAG

## Work Items (in execution order)

### ✅ WI-1: Fix VectorStore.Upsert interface + Qdrant impl (DONE)
- `interface.go` — `Upsert(ctx, docs, embeddings)` now accepts parallel embeddings slice
- `qdrant.go` — attaches `Vectors` to each `PointStruct` before upsert
- `pipeline.go` — removed bridge method, calls `store.Upsert` directly with embeddings

### WI-2: Fix LLM Pinger — zero-cost health check
**Problem:** `LLMPinger.Ping()` calls `model.Generate("ping")` — a full
completion request that consumes tokens on every readiness probe. At 5s
probe intervals, this silently burns tokens 24/7.

**Solution by provider:**
| Provider     | Zero-cost probe method                              |
|-------------|------------------------------------------------------|
| Ollama      | `GET /api/tags` — list models, confirms server alive |
| OpenAI      | `GET /v1/models` — list models, confirms API key valid |
| Azure OpenAI | `GET /openai/deployments?api-version=...` — confirms endpoint + key |
| Bedrock     | `ListFoundationModels` API — no inference cost       |
| Gemini      | `ListModels` — no inference cost                     |

**Implementation:**
- Add a `HealthChecker` interface to the provider package:
  ```go
  type HealthChecker interface {
      HealthCheck(ctx context.Context) error
  }
  ```
- Each provider backend exports a lightweight `HealthCheck()` method using
  its native list-models or metadata endpoint — no tokens consumed.
- `LLMPinger` accepts `HealthChecker` instead of `model.ToolCallingChatModel`.
- If the provider doesn't implement `HealthChecker`, fall back to a TCP dial
  against the endpoint (confirms network reachability without token cost).

**Files:**
- `internal/provider/interface.go` — add `HealthChecker` interface
- `internal/provider/backends.go` — implement per-provider health checks
- `internal/server/pingers.go` — `LLMPinger` uses `HealthChecker`
- `cmd/tfai/commands/helpers.go` — wire the new pinger

### WI-3: Provider-agnostic Embedder — cascading factory (mirrors `provider.NewFromEnv()`)
**Problem:** No concrete `rag.Embedder` implementation exists. Need one that
supports Azure OpenAI embeddings today, OpenAI/Bedrock/Gemini tomorrow —
without coupling the ingestion pipeline to any single provider.

**Design:**
```
rag.Embedder (interface — already exists)
  ├── embedder.OpenAIEmbedder    — OpenAI + Azure OpenAI (text-embedding-3-small)
  ├── embedder.OllamaEmbedder    — Ollama (nomic-embed-text / mxbai-embed-large)
  ├── embedder.BedrockEmbedder   — future: Titan Embeddings v2
  └── embedder.GeminiEmbedder    — future: text-embedding-004
```

**Cascading default resolution (fail-soft):**
```
1. Read EMBEDDING_PROVIDER env var
2. If unset → inherit MODEL_PROVIDER (you're embedding with the same backend)
3. Switch on resolved provider:
     ┌─────────┬───────────────────────────────────┬─────────────────────────────┐
     │ Backend │ Inherited credentials              │ Default embedding model     │
     ├─────────┼───────────────────────────────────┼─────────────────────────────┤
     │ ollama  │ OLLAMA_HOST                        │ nomic-embed-text            │
     │ openai  │ OPENAI_API_KEY                     │ text-embedding-3-small      │
     │ azure   │ AZURE_OPENAI_API_KEY,              │ text-embedding-3-small      │
     │         │ AZURE_OPENAI_ENDPOINT              │                             │
     │ bedrock │ AWS credential chain, AWS_REGION   │ amazon.titan-embed-text-v2  │
     │ gemini  │ GOOGLE_API_KEY                     │ text-embedding-004          │
     └─────────┴───────────────────────────────────┴─────────────────────────────┘
4. Any EMBEDDING_* override takes precedence over inherited values:
     EMBEDDING_MODEL      → overrides the default model for that backend
     EMBEDDING_API_KEY    → overrides the inherited API key
     EMBEDDING_ENDPOINT   → overrides the inherited endpoint
     EMBEDDING_DIMENSIONS → overrides the default dimensions (default: 1536)
```

**Key behavior:** If you set nothing beyond `MODEL_PROVIDER=azure`, the
embedder uses the same Azure endpoint and key with `text-embedding-3-small`.
If you want a different provider for embeddings, set `EMBEDDING_PROVIDER`
and the relevant `EMBEDDING_*` vars — the chat config is untouched. This
gives full mix-and-match (Azure chat + OpenAI embeddings, Ollama chat +
Azure embeddings, etc.) with zero config when the provider is the same.

**Implementation:**
- New package: `internal/embedder/`
- `internal/embedder/openai.go` — implements `rag.Embedder` via the
  OpenAI `/v1/embeddings` REST API. Works for both OpenAI and Azure OpenAI
  (same API shape, Azure uses `ByAzure` base URL + api-version param).
  No new deps — just `net/http` + JSON.
- `internal/embedder/ollama.go` — implements `rag.Embedder` via the
  Ollama `/api/embeddings` REST API. Local-first, no API key needed.
- `internal/embedder/factory.go` — `NewFromEnv()` factory: reads
  `EMBEDDING_PROVIDER`, falls back to `MODEL_PROVIDER`, switches on backend,
  inherits credentials, applies `EMBEDDING_*` overrides. Mirrors the
  `provider.NewFromEnv()` pattern exactly.

**Files:**
- `internal/embedder/openai.go` — OpenAI + Azure OpenAI embedder
- `internal/embedder/ollama.go` — Ollama embedder
- `internal/embedder/factory.go` — `NewFromEnv()` cascading factory

### WI-4: Wire `ingest.go` to real embedder + Qdrant
**Problem:** `ingest.go` is a stub that prints TODO and exits.

**Implementation:**
- Construct `rag.Embedder` via `embedder.NewFromEnv()`
- Construct `rag.QdrantStore` from env vars (`QDRANT_HOST`, `QDRANT_PORT`,
  `QDRANT_COLLECTION`, `EMBEDDING_DIMENSIONS`)
- Construct `ingestion.Pipeline` and call `Ingest()`
- Auto-detect `ResourceType` from URL path (e.g.
  `.../resources/eks_cluster` → `aws_eks_cluster`)
- Progress output to stderr

**Files:**
- `cmd/tfai/commands/ingest.go` — replace stub with real wiring
- `.env.example` — document new env vars

### WI-5: Expand ingest sources (EKS-critical minimum)
**Problem:** Current AWS sources are 3 URLs. For EKS quality improvement we
need the resources the model is currently missing.

**Minimum viable AWS EKS corpus:**
```
aws_eks_cluster
aws_eks_node_group
aws_eks_addon
aws_iam_openid_connect_provider
aws_kms_key
aws_security_group / aws_security_group_rule
aws_launch_template
aws_iam_role
aws_iam_role_policy_attachment
```

**Files:**
- `Makefile` — expand `ingest-aws` target with full EKS resource set

### WI-6: Wire retriever into agent for `generate` and `ask`
**Problem:** The retriever exists but is it actually wired into the agent's
message pipeline? Need to verify and fix if not.

**Check:** Does `agent.buildMessages()` actually call the retriever and inject
RAG context? If yes, just ensure the Qdrant config is passed through from
`serve` and `generate` commands. If not, wire it.

**Files:**
- `cmd/tfai/commands/serve.go` — pass retriever to agent config
- `cmd/tfai/commands/generate.go` — pass retriever to agent config
- `cmd/tfai/commands/helpers.go` — build retriever from env vars

### WI-7: End-to-end validation
1. Start Qdrant: `docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant`
2. Ingest: `make ingest-aws`
3. Verify vectors: query Qdrant API for collection point count
4. Generate EKS module: `make fs/v2-with-rag && ./bin/tfai generate --out test/v2-with-rag "production EKS cluster"`
5. Compare `test/v2-sysprompt-no-rag/` vs `test/v2-with-rag/`
6. Confirm: `encryption_config`, OIDC provider, managed add-ons now present
7. `make gate`

## Env Var Summary

```bash
# ── Qdrant ──
QDRANT_HOST=localhost          # Qdrant server hostname (default: localhost)
QDRANT_PORT=6334               # Qdrant gRPC port (default: 6334)
QDRANT_COLLECTION=tfai-docs    # Collection name (default: tfai-docs)

# ── Embedding (cascading defaults — inherits from chat provider) ──
# If EMBEDDING_PROVIDER is unset, it inherits MODEL_PROVIDER.
# If EMBEDDING_API_KEY is unset, it inherits the chat provider's key.
# If EMBEDDING_ENDPOINT is unset, it inherits the chat provider's endpoint.
# Only set EMBEDDING_* vars when you want to diverge from the chat provider.
#
# Example: same provider for everything (zero extra config)
#   MODEL_PROVIDER=azure  → embedder uses Azure endpoint + key + text-embedding-3-small
#
# Example: split providers (chat=Azure, embeddings=OpenAI)
#   MODEL_PROVIDER=azure
#   EMBEDDING_PROVIDER=openai
#   EMBEDDING_API_KEY=sk-...
#
EMBEDDING_PROVIDER=            # Override: ollama | openai | azure | bedrock | gemini
EMBEDDING_MODEL=               # Override: model name (default per backend, see WI-3 table)
EMBEDDING_API_KEY=             # Override: API key (inherits from chat provider if unset)
EMBEDDING_ENDPOINT=            # Override: base URL (inherits from chat provider if unset)
EMBEDDING_DIMENSIONS=1536      # Vector dimensions (must match Qdrant collection)
```

## Execution Order

```
WI-1: Fix Upsert interface + Qdrant impl          ✅ DONE
  ↓
WI-2: Fix LLM Pinger (zero-cost health check)
  ↓
WI-3: Embedder interface + OpenAI/Azure impl
  ↓
WI-4: Wire ingest.go to real embedder + Qdrant
  ↓
WI-5: Expand ingest sources
  ↓
WI-6: Wire retriever into agent
  ↓
WI-7: End-to-end validation
  ↓
make gate → commit → push → PR
```

## Out of Scope (this PR)

- Bedrock/Gemini embedder implementations (interface is ready, impls come later)
- Auto-ingest on demand (Phase 5)
- Chunk size tuning (needs eval framework first)
- YAML config for RAG settings (Phase 2.5)
