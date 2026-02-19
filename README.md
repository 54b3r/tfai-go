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
# 1. Copy and configure your environment
cp .env.example .env
# Edit .env — set MODEL_PROVIDER and credentials

# 2. Start supporting services (Qdrant + Langfuse)
make up

# 3. Build and run
make run

# Or run the full stack in Docker
make run-docker
```

The web UI is available at **http://localhost:8080** after `make run` or `make run-docker`.

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

# Ingest provider documentation into the RAG store
tfai ingest --provider aws \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster
```

---

## Model Provider Configuration

Set `MODEL_PROVIDER` to select your inference backend. All other config is via env vars.

| Provider | `MODEL_PROVIDER` | Required env vars |
|---|---|---|
| **Ollama** (local) | `ollama` | `MODEL_BASE_URL` (default: `http://localhost:11434`), `MODEL_NAME` |
| **OpenAI** | `openai` | `MODEL_API_KEY`, `MODEL_NAME` |
| **Azure OpenAI** | `azure` | `MODEL_API_KEY`, `MODEL_BASE_URL` (endpoint), `AZURE_DEPLOYMENT` |
| **AWS Bedrock** | `bedrock` | AWS credential chain, `MODEL_NAME`, `AWS_REGION` |
| **Google Gemini** | `gemini` | `MODEL_API_KEY`, `MODEL_NAME` |

See `.env.example` for the full reference.

---

## Architecture

```
tfai-go/
├── cmd/tfai/                   # Cobra CLI entrypoint + commands
│   └── commands/               # ask, generate, diagnose, serve, ingest
├── internal/
│   ├── agent/                  # Eino ReAct agent + RAG context injection
│   ├── provider/               # ChatModel factory (interface + backends)
│   ├── tools/                  # Terraform tools: plan, state, generate
│   ├── rag/                    # VectorStore + Embedder + Retriever interfaces
│   │                           # Qdrant implementation
│   ├── ingestion/              # Doc fetch → chunk → embed → upsert pipeline
│   └── server/                 # HTTP server + SSE streaming + web UI
├── ui/static/                  # Web UI (served by tfai serve)
├── Dockerfile
├── docker-compose.yml          # app + qdrant + langfuse
└── Makefile                    # 3 Musketeers targets
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
make fmt            # Run gofmt + goimports
make ingest-aws     # Ingest core AWS provider docs
make ingest-azure   # Ingest core Azure provider docs
make ingest-gcp     # Ingest core GCP provider docs
make clean          # Remove build artifacts
```

---

## License

Apache 2.0
