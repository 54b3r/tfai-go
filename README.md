# tfai-go

> **TF-AI** — A local-first AI Terraform expert agent for platform engineers and consultants.

Generate infrastructure code, diagnose failures, and get expert guidance across AWS, Azure, and GCP — powered by any LLM backend you already have access to.

---

## What it does

- **Generate** production-grade Terraform HCL from natural language
- **Diagnose** `terraform plan` / `apply` failures with root-cause analysis
- **Inspect** state files, detect drift, advise on recovery
- **Design** multi-cloud modules (EKS, AKS, GKE, AI platforms, networking)
- **RAG-backed** — ingest Terraform provider docs for accurate, hallucination-resistant answers
- **Multi-provider** — swap inference backends via a single env var

---

## Quick Start

```bash
# 1. Configure settings (model, server, qdrant, etc.)
cp config.yaml.example config.yaml
# Edit config.yaml — set model.provider and other non-secret settings

# 2. Add secrets (API keys only)
cp .env.example .env
# Edit .env — uncomment and set API keys for your provider

# 3. Start supporting services (Qdrant + Langfuse)
make up

# 4. Build and run
make run

# Or run the full stack in Docker
make run-docker
```

The web UI is available at **http://localhost:8080** after `make run` or `make run-docker`.

> **Configuration model**: `config.yaml` is the primary configuration file
> (cloud-native standard). `.env` holds secrets only (API keys, tokens).
> Environment variables override `config.yaml` values when set.

---

## CLI Usage

```bash
# Ask a question
tfai ask "how do I create an EKS cluster with IRSA and private endpoints?"

# Generate Terraform files into a directory
tfai generate --out ./infra/eks "EKS cluster with managed node groups, IRSA, and private API endpoint"

# Diagnose a plan failure (pipe or file)
terraform plan 2>&1 | tfai diagnose
tfai diagnose --plan ./plan.txt

# Diagnose by running plan directly
tfai diagnose --dir ./infra/eks

# Start the web UI server
tfai serve --port 8080

# Ingest provider documentation into the RAG store (metadata auto-inferred from URL)
tfai ingest --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster

# Override inferred metadata for custom/internal docs
tfai ingest --provider aws --framework terraform --doc-type guide \
  --url https://internal.wiki.example.com/aws-best-practices
```

---

## Configuration

tfai uses **YAML as the primary configuration format** (the cloud-native standard).
Secrets (API keys, tokens) go in `.env` or environment variables — never in YAML.

**Precedence**: env vars > YAML file > built-in defaults

### Config file search order

1. `--config <path>` CLI flag (explicit)
2. `TFAI_CONFIG` environment variable
3. `~/.tfai/config.yaml`
4. `./tfai.yaml`

### Example `config.yaml`

```yaml
model:
  provider: azure
  max_tokens: 8192
  temperature: 0.3
  azure:
    endpoint: https://my-resource.openai.azure.com
    deployment: gpt-4o

embedding:
  provider: ollama
  model: nomic-embed-text

qdrant:
  host: qdrant.internal
  port: 6334
  collection: my-docs

server:
  host: 127.0.0.1
  port: 8080

logging:
  level: info
  format: json
```

See `config.yaml.example` for the full annotated reference with all sections.

### Model providers

Set `model.provider` in `config.yaml` to select your inference backend:

| Provider | `model.provider` | Config section | Secret (`.env`) |
|---|---|---|---|
| **Ollama** (local) | `ollama` | `model.ollama.host`, `model.ollama.model` | — |
| **OpenAI** | `openai` | `model.openai.model` | `OPENAI_API_KEY` |
| **Azure OpenAI** | `azure` | `model.azure.endpoint`, `model.azure.deployment` | `AZURE_OPENAI_API_KEY` |
| **AWS Bedrock** | `bedrock` | `model.bedrock.region`, `model.bedrock.model_id` | AWS credential chain |
| **Google Gemini** | `gemini` | `model.gemini.model` | `GOOGLE_API_KEY` |

### Secrets

API keys belong in `.env` (or injected as environment variables in CI/CD),
**never** in `config.yaml`:

```bash
# .env — secrets only, never committed
OPENAI_API_KEY=sk-...
AZURE_OPENAI_API_KEY=...
TFAI_API_KEY=...          # enables Bearer auth on API endpoints
```

Environment variables override any value in `config.yaml`.

---

## Audit Logging

Every CLI command emits a structured JSON audit log entry at startup:

```json
{
  "level": "INFO",
  "msg": "audit: command start",
  "command": "serve",
  "config_file": "~/.tfai/config.yaml",
  "MODEL_PROVIDER": "azure",
  "OPENAI_API_KEY": "unset",
  "AZURE_OPENAI_API_KEY": "set",
  "QDRANT_HOST": "localhost"
}
```

**Key sanitisation**: Secret values (API keys, tokens) are logged as `"set"` or
`"unset"` only — never the actual value. Non-secret config (provider, host,
port, model) is logged in full for operational visibility.

The audit trail is emitted via `slog` and respects `LOG_LEVEL` / `LOG_FORMAT`.

---

## Architecture

```
tfai-go/
├── cmd/tfai/                   # Cobra CLI entrypoint + commands
│   └── commands/               # ask, generate, diagnose, serve, ingest
├── internal/
│   ├── agent/                  # Eino ReAct agent + RAG context injection
│   ├── audit/                  # Structured audit logger with key sanitisation
│   ├── config/                 # YAML config loader (layered: defaults → YAML → env)
│   ├── provider/               # ChatModel factory (interface + backends)
│   ├── tools/                  # Terraform tools: plan, state, generate
│   ├── rag/                    # VectorStore + Embedder + Retriever interfaces
│   │                           # Qdrant implementation
│   ├── ingestion/              # Doc fetch → chunk → embed → upsert pipeline
│   └── server/                 # HTTP server + SSE streaming + web UI
├── ui/static/                  # Web UI (served by tfai serve)
├── .golangci.yml               # golangci-lint config (15 linters incl. gosec)
├── .windsurf/rules/            # Project coding + security/SRE rules
├── Dockerfile
├── docker-compose.yml          # app + qdrant + langfuse
└── Makefile                    # build, test, lint, gate targets
```

**Key design decisions:**
- `provider.Factory` interface — swap LLM backends without touching agent code
- `rag.VectorStore` / `rag.Retriever` interfaces — swap vector DB without touching agent code
- `tools.Runner` interface — inject fake runner in tests, no real terraform binary needed
- Eino `react.Agent` handles the full ReAct loop — tool selection, execution, response

---

## Stack

| Component | Technology |
|---|---|
| Language | Go 1.26 |
| LLM framework | [Eino](https://github.com/cloudwego/eino) (CloudWeGo) |
| Vector store | [Qdrant](https://qdrant.tech) |
| Observability | [Langfuse](https://langfuse.com) |
| CLI | [Cobra](https://github.com/spf13/cobra) |

---

## Makefile Targets

```bash
make help           # Show all targets
make deps           # Download Go dependencies
make build          # Build tfai binary to bin/
make run            # Build + run tfai serve
make up             # Start qdrant + langfuse in Docker
make run-docker     # Full stack via docker compose
make test           # Run unit tests
make lint           # Run golangci-lint
make lint-fix       # Run golangci-lint with auto-fix
make fmt            # Run gofmt + goimports
make gate           # Full pre-commit gate: build + vet + lint + vulncheck + test + binary smoke
make install-tools  # Install dev tools (golangci-lint, goimports, govulncheck)
make ingest-aws     # Ingest core AWS provider docs
make ingest-azure   # Ingest core Azure provider docs
make ingest-gcp     # Ingest core GCP provider docs
make ingest-atmos   # Ingest Atmos framework docs
make ingest-all     # Ingest all provider + framework docs
make clean          # Remove build artifacts
```

---

## API Reference

### Authentication

Set `TFAI_API_KEY` to enable Bearer token authentication. When set, all
`/api/*` routes except `/api/health` and `/api/ready` require:

```
Authorization: Bearer <TFAI_API_KEY>
```

If `TFAI_API_KEY` is unset the server starts in **unauthenticated mode** with a
startup warning — suitable for local development only.

### Endpoints

| Method | Path | Auth | Rate limited | Description |
|---|---|---|---|---|
| `GET` | `/api/health` | No | No | Liveness — always 200 while process is running |
| `GET` | `/api/ready` | No | No | Readiness — probes LLM + Qdrant, returns 200 or 503 |
| `GET` | `/api/config` | No | No | UI bootstrap — returns `{"auth_required": true/false}` |
| `POST` | `/api/chat` | Yes | Yes | Stream agent response (SSE) |
| `GET` | `/api/workspace` | Yes | Yes | List workspace files and metadata |
| `POST` | `/api/workspace/create` | Yes | Yes | Scaffold a new workspace |
| `GET` | `/api/file` | Yes | Yes | Read a file |
| `PUT` | `/api/file` | Yes | Yes | Write a file |
| `GET` | `/metrics` | No | No | Prometheus metrics scrape endpoint |

### Rate limiting

Per-IP token bucket: **10 requests/second sustained, burst 20** (defaults).
Exceeded requests receive `429 Too Many Requests` with a `Retry-After: 1` header.

### Readiness response

```json
{
  "ready": false,
  "checks": [
    {"name": "ollama", "ok": false, "error": "model not found"},
    {"name": "qdrant",  "ok": true}
  ]
}
```

---

## RAG Ingestion & Metadata

### Auto-inferred metadata

When you run `tfai ingest --url <URL>`, the pipeline automatically infers
`provider`, `framework`, and `doc_type` from the URL pattern. This metadata is
stored as Qdrant payload fields on every chunk, enabling filtered retrieval.

```bash
# These two commands produce identical metadata — the second infers it:
tfai ingest --provider aws --framework terraform --doc-type reference \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster

tfai ingest \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster
# → provider=aws, framework=terraform, doc_type=reference
```

Explicit flags (`--provider`, `--framework`, `--doc-type`) **always override**
inferred values. Use them for custom or internal documentation URLs that don't
match any known pattern.

### Supported URL patterns

| URL host | Example path | Inferred framework | Inferred provider | Inferred doc_type |
|---|---|---|---|---|
| `registry.terraform.io` | `/providers/hashicorp/aws/.../resources/...` | `terraform` | `aws` | `reference` |
| `registry.terraform.io` | `/providers/hashicorp/azurerm/.../guides/...` | `terraform` | `azure` | `guide` |
| `registry.terraform.io` | `/providers/hashicorp/google/.../data-sources/...` | `terraform` | `gcp` | `reference` |
| `atmos.tools` | `/core-concepts/...` | `atmos` | `atmos` | `reference` |
| `atmos.tools` | `/quick-start/...` | `atmos` | `atmos` | `tutorial` |
| `atmos.tools` | `/integrations/...` | `atmos` | `atmos` | `guide` |
| `developer.hashicorp.com` | `/terraform/tutorials/...` | `terraform` | `generic` | `tutorial` |
| `developer.hashicorp.com` | `/terraform/language/...` | `terraform` | `generic` | `reference` |
| `terragrunt.gruntwork.io` | `/docs/...` | `terragrunt` | `generic` | `reference` |
| *(unknown)* | — | `terraform` | `generic` | `reference` |

### Provider alias mapping

Terraform Registry provider names are mapped to canonical short labels:

| Registry name | Canonical label |
|---|---|
| `aws` | `aws` |
| `azurerm`, `azuread` | `azure` |
| `google`, `google-beta` | `gcp` |
| `kubernetes`, `helm` | `kubernetes` |
| `random`, `null`, `local`, `tls`, ... | `generic` |

Unknown provider names are used as-is (e.g. `datadog` → `datadog`).

### Adding a new URL pattern

To support a new documentation source:

1. **Edit** `internal/ingestion/metadata.go`
2. **Add a case** in `InferMetadata()` matching the new host/path
3. **Add a test** in `internal/ingestion/metadata_test.go`
4. **Run** `make gate` to verify

For new Terraform Registry providers, just add an entry to `registryProviderAliases`
in `metadata.go` — no other code changes needed.

### Future: LLM-based classification

For URLs that don't match any pattern, a future `--classify` flag will invoke a
lightweight model to infer metadata from the page content. This is planned but
not yet implemented. Until then, pass explicit flags for custom URLs.

---

## Security Model

tfai binds to `127.0.0.1` by default and is designed for **single-user local use**.

| Threat | Mitigation |
|---|---|
| Unauthenticated API access | Bearer token auth on all `/api/*` routes (opt-in via `TFAI_API_KEY`) |
| Request flood / DoS | Per-IP token-bucket rate limiting (10 rps, burst 20) on all API routes |
| Path traversal via LLM output | All file writes confined to declared workspace root |
| Path traversal via API params | `confineToDir` enforced on all file API calls |
| Arbitrary directory creation | `POST /api/workspace/create` requires pre-existing directory |
| Oversized request DoS | `http.MaxBytesReader` (1 MiB) on `/api/chat` |
| Secret leakage | Credentials only from env vars, never logged or returned |
| Prompt injection via workspace | Only `.tf` files injected into LLM context |

See `.windsurf/rules/` for the full coding, SRE, and security policy.

---

## Documentation

| Document | Description |
|---|---|
| [docs/BACKSTAGE.md](docs/BACKSTAGE.md) | Backstage integration — catalog registration, scaffolder template, deployment guide |
| [docs/TESTING.md](docs/TESTING.md) | Manual testing & smoke test guide — step-by-step verification of every feature |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Unified 3-tier roadmap (Immediate → Medium → Complete) |
| [docs/REVIEW.md](docs/REVIEW.md) | Full codebase review and architecture scorecard |
| [docs/SRE_ASSESSMENT.md](docs/SRE_ASSESSMENT.md) | SRE readiness assessment — profiling, security, logging, observability |
| [docs/STRATEGIC_ANALYSIS.md](docs/STRATEGIC_ANALYSIS.md) | Strategic analysis — accelerator vs product, MCP evaluation |
| [config.yaml.example](config.yaml.example) | Full YAML configuration reference with all sections |
| [catalog-info.yaml](catalog-info.yaml) | Backstage catalog entity descriptor (Component + API + Resource) |
| [.env.example](.env.example) | Environment variable reference with per-provider examples |

---

## License

Apache 2.0
