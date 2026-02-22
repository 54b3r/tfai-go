# ${{ values.instanceName }}

> ${{ values.description }}

Powered by [TF-AI](https://github.com/54b3r/tfai-go) — a local-first AI Terraform expert agent.

## Quick Start

```bash
# 1. Review config.yaml — non-secret settings are pre-configured
#    Edit config.yaml to adjust model, embedding, qdrant, or server settings

# 2. Add secrets (API keys only)
cp .env.example .env
# Edit .env — uncomment and set API keys for your provider

# 3. Start services
make up

# 4. Start TF-AI
make run

# 5. Open the web UI
open http://localhost:${{ values.serverPort }}
```

## Configuration

**`config.yaml`** is the primary configuration file (cloud-native standard).
**`.env`** holds secrets only (API keys, tokens) — never commit this file.
Environment variables override `config.yaml` values when set.

| Setting | Value | Source |
|---|---|---|
| Model Provider | `${{ values.modelProvider }}` | config.yaml |
| Server Port | `${{ values.serverPort }}` | config.yaml |
| RAG (Qdrant) | `${{ values.enableRag }}` | config.yaml |
| Authentication | `${{ values.enableAuth }}` | .env |
| Tracing (Langfuse) | `${{ values.enableTracing }}` | config.yaml |
| Owner | `${{ values.owner }}` | catalog-info.yaml |

## Useful Commands

```bash
make up           # Start supporting services
make run          # Start TF-AI
make down         # Stop all services
make logs         # Tail logs
make ingest-all   # Ingest provider docs into Qdrant
```

## Documentation

- [TF-AI README](https://github.com/54b3r/tfai-go/blob/main/README.md)
- [Backstage Integration Guide](https://github.com/54b3r/tfai-go/blob/main/docs/BACKSTAGE.md)
- [Testing Guide](https://github.com/54b3r/tfai-go/blob/main/docs/TESTING.md)
