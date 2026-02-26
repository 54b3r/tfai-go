# Changelog

All notable changes to TF-AI-Go are documented here.

This project follows [Semantic Versioning](https://semver.org/) and uses the
[Keep a Changelog](https://keepachangelog.com/) format.

For detailed planning and roadmap, see [docs/ROADMAP.md](docs/ROADMAP.md).

---

## [Unreleased]

### Added
- Azure Codex support for GPT-5.2-Codex via `/openai/responses` endpoint (#63)

---

## [0.29.0] - 2026-02-25

### Added
- Binary smoke tests in CI workflow
- RC (release candidate) release support

### Fixed
- Smoke test regressions: double path in generate, version PersistentPreRunE skip
- Makefile install target
- config.yaml.example updates

### Changed
- Updated TESTING.md with mkdir instructions

**PRs:** #59, #62

---

## [0.28.0] - 2026-02-24

### Added
- Generate model override: separate LLM for code generation via `GENERATE_*` env vars
- golangci-lint v2 migration

**PRs:** #58

---

## [0.27.0] - 2026-02-23

### Added
- Per-IP rate limiting
- Request ID header for tracing
- Audit log on CLI startup
- Structured startup/shutdown logging

**PRs:** #57

---

## [0.25.0] - 2026-02-22

### Added
- Backstage integration: catalog entity, scaffolder template
- YAML-first config shift

**PRs:** #49

---

## [0.24.0] - 2026-02-22

### Added
- YAML config file support (`internal/config`)
- Structured CLI audit logging (`internal/audit`)

**PRs:** #48

---

## [0.23.0] - 2026-02-22

### Added
- RAG metadata auto-inference from URLs
- Expanded Makefile ingest targets
- Structured Qdrant payload

**PRs:** #45

---

## [0.22.0] - 2026-02-21

### Added
- Embedder config guardrails with fail-fast validation

### Fixed
- QDRANT_PORT configuration
- Nil-guard for qdrant client

**PRs:** #43, #44

---

## [0.21.0] - 2026-02-21

### Added
- RAG pipeline wired end-to-end: ingest → embed → store → retrieve → serve

**PRs:** #43

---

## [0.20.0] - 2026-02-21

### Added
- RAG foundation: embedder factory, VectorStore.Upsert
- Zero-cost LLM health checks

**PRs:** #43

---

## [0.19.0] - 2026-02-20

### Added
- System prompt v2
- Security hardening: constant-time auth, tool audit logging
- govulncheck integration
- SRE assessment documentation

### Fixed
- Azure reasoning model configuration

**PRs:** #39, #41, #42

---

## [0.18.0] - 2026-02-20

### Added
- Token budget management
- Codebase review documentation
- Strategic analysis documentation

**PRs:** #38

---

## [0.17.0] - 2026-02-20

### Added
- Conversation history persistence (SQLite)

**PRs:** #37

---

## [0.16.0 and earlier] - Pre-2026-02-20

### Added
- Core agent implementation
- `serve`, `ask`, `generate` commands
- Web UI
- Terraform tools (plan, state, generate)
- Prometheus metrics endpoint
- Authentication middleware
- Rate limiting
- Health/readiness probes
- Structured logging with slog
- Graceful shutdown

---

[Unreleased]: https://github.com/54b3r/tfai-go/compare/v0.29.0...HEAD
[0.29.0]: https://github.com/54b3r/tfai-go/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/54b3r/tfai-go/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/54b3r/tfai-go/compare/v0.25.0...v0.27.0
[0.25.0]: https://github.com/54b3r/tfai-go/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/54b3r/tfai-go/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/54b3r/tfai-go/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/54b3r/tfai-go/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/54b3r/tfai-go/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/54b3r/tfai-go/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/54b3r/tfai-go/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/54b3r/tfai-go/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/54b3r/tfai-go/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/54b3r/tfai-go/releases/tag/v0.16.0
