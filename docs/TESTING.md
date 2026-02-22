# TF-AI-Go — Manual Testing & Smoke Test Guide

**Purpose:** Step-by-step guide for verifying every feature of tfai-go after any code change. Designed to be followed without AI assistance.

**Last updated:** 2026-02-22 (v0.20.x — RAG pipeline wired)

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Build & Gate Verification](#2-build--gate-verification)
3. [CLI Smoke Tests](#3-cli-smoke-tests)
4. [Server Startup & Health](#4-server-startup--health)
5. [API Endpoint Tests](#5-api-endpoint-tests)
6. [Web UI Smoke Tests](#6-web-ui-smoke-tests)
7. [Authentication Tests](#7-authentication-tests)
8. [Rate Limiting Tests](#8-rate-limiting-tests)
9. [Observability Verification](#9-observability-verification)
10. [Docker Compose Stack](#10-docker-compose-stack)
11. [Conversation History Tests](#11-conversation-history-tests)
12. [Security Regression Tests](#12-security-regression-tests)
13. [Graceful Shutdown Test](#13-graceful-shutdown-test)
14. [Known Limitations](#14-known-limitations)
15. [RAG Pipeline Smoke Tests](#15-rag-pipeline-smoke-tests)

---

## 1. Prerequisites

### Required

- **Go 1.26+**: `go version`
- **golangci-lint**: `golangci-lint --version`
- **govulncheck**: `govulncheck -version` (install: `go install golang.org/x/vuln/cmd/govulncheck@latest`)
- **A configured LLM provider** (see `.env.example`). Ollama is easiest for local testing.

### Optional (for full stack)

- **Docker** + **Docker Compose**: for Qdrant, Langfuse, containerised tfai
- **Terraform**: for `terraform_plan` and `terraform_state` tool testing
- **curl** or **httpie**: for API endpoint testing

### Environment Setup

```bash
# Clone and enter the repo
cd /path/to/tfai-go

# Copy env template and configure your provider
cp .env.example .env
# Edit .env — at minimum set MODEL_PROVIDER and model-specific vars

# Install dev tools (one-time)
make install-tools

# Download Go dependencies
make deps
```

---

## 2. Build & Gate Verification

The gate is the single source of truth. **Every change must pass the gate before committing.**

```bash
make gate
```

### What the gate runs (in order)

| Step | Command | What it checks |
|---|---|---|
| build | `go build -trimpath -ldflags=... -o bin/tfai ./cmd/tfai` | Compiles without errors |
| vet | `go vet ./...` | Static analysis (unused vars, bad formatting, etc.) |
| lint | `golangci-lint run ./...` | 15 linters including gosec, errcheck, wrapcheck |
| vulncheck | `govulncheck ./...` | Known CVEs in dependencies |
| test | `go test -race -count=1 ./...` | All unit tests with race detector |
| binary smoke | `./bin/tfai version` | Binary runs, version info present |
| binary smoke | `./bin/tfai --help` | Help text renders |
| binary smoke | `./bin/tfai serve --help` | Serve subcommand help renders |

### Expected output

```
── gate: build ──────────────────────────────────────────
── gate: vet ────────────────────────────────────────────
── gate: lint ───────────────────────────────────────────
── gate: vulncheck ──────────────────────────────────────
=== Symbol Results ===
No vulnerabilities found.
── gate: test ───────────────────────────────────────────
ok  github.com/54b3r/tfai-go/internal/agent    ...
ok  github.com/54b3r/tfai-go/internal/budget   ...
ok  github.com/54b3r/tfai-go/internal/provider ...
ok  github.com/54b3r/tfai-go/internal/server   ...
ok  github.com/54b3r/tfai-go/internal/store    ...
── gate: binary smoke ───────────────────────────────────
── gate: PASS ───────────────────────────────────────────
```

### If the gate fails

1. Read the failing step's output carefully
2. For lint failures: `make lint-fix` may auto-fix some issues
3. For test failures: `make test-verbose` shows detailed test output
4. For build failures: check your Go version (`go version`) and run `make deps`

### Running individual steps

```bash
make build          # just build
make test           # just tests
make test-verbose   # tests with -v
make test-cover     # tests with coverage report → coverage.html
make lint           # just lint
make fmt            # format all files (gofmt + goimports)
```

---

## 3. CLI Smoke Tests

These tests verify each CLI command works end-to-end. **Requires a configured LLM provider.**

### 3.1 Version

```bash
./bin/tfai version
```

**Expected:** Version string with commit hash and build date:
```
tfai v0.18.0 (commit: abc1234, built: 2026-02-20T22:00:00Z)
```

### 3.2 Ask

```bash
./bin/tfai ask "what is a terraform backend?"
```

**Expected:** Streaming text response to stdout. Should mention remote state storage, S3/GCS/Azure blob, etc.

### 3.3 Ask with workspace context

```bash
# Create a test workspace first
mkdir -p /tmp/tfai-test-ws
echo 'resource "aws_s3_bucket" "test" { bucket = "my-bucket" }' > /tmp/tfai-test-ws/main.tf

./bin/tfai ask --dir /tmp/tfai-test-ws "what resources are defined in my workspace?"
```

**Expected:** Response references the S3 bucket resource.

### 3.4 Generate

```bash
./bin/tfai generate --out /tmp/tfai-gen-test "S3 bucket with versioning and server-side encryption"
```

**Expected:**
- `.tf` files written to `/tmp/tfai-gen-test/`
- Files should include `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`
- Content should contain `aws_s3_bucket` and `aws_s3_bucket_versioning` resources

```bash
# Verify files were created
ls -la /tmp/tfai-gen-test/
cat /tmp/tfai-gen-test/main.tf
```

### 3.5 Diagnose (with piped input)

```bash
echo 'Error: creating S3 Bucket (my-bucket): BucketAlreadyExists' | ./bin/tfai diagnose
```

**Expected:** Response diagnosing the bucket name conflict with remediation steps.

### 3.6 Diagnose (with file)

```bash
echo 'Error: creating EC2 Instance: UnauthorizedAccess' > /tmp/plan-error.txt
./bin/tfai diagnose --plan /tmp/plan-error.txt
```

**Expected:** Response about IAM permissions and how to fix them.

### 3.7 Ingest (requires Qdrant + embedder)

```bash
# Without Qdrant running — expect a clear connection error
./bin/tfai ingest --provider aws \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket
```

**Expected (Qdrant not running):**
```
time=... level=INFO msg="embedder initialised" provider=ollama
time=... level=ERROR msg="ingest: failed to connect to Qdrant at localhost:6334: ..."
```

**Expected (Qdrant running — see section 15 for full RAG smoke test):**
```
time=... level=INFO msg="embedder initialised" provider=ollama
time=... level=INFO msg="qdrant store ready" host=localhost port=6334 collection=tfai-docs
time=... level=INFO msg="starting ingestion" sources=1 provider=aws
time=... level=INFO msg="fetching https://..."
time=... level=INFO msg="chunked ... into N chunks"
time=... level=INFO msg="ingested N chunks from https://..."
time=... level=INFO msg="ingestion complete" sources=1
```

### Cleanup

```bash
rm -rf /tmp/tfai-test-ws /tmp/tfai-gen-test /tmp/plan-error.txt
```

---

## 4. Server Startup & Health

### 4.1 Start the server

```bash
# Terminal 1: start the server
./bin/tfai serve
```

**Expected log output:**
```
{"level":"INFO","msg":"serve starting","provider":"ollama"}
{"level":"WARN","msg":"auth disabled: TFAI_API_KEY not set — all API routes are unauthenticated"}
{"level":"INFO","msg":"provider initialised","provider":"ollama"}
{"level":"INFO","msg":"server listening","addr":"http://127.0.0.1:8080"}
```

### 4.2 Liveness probe

```bash
curl -s http://localhost:8080/api/health | jq .
```

**Expected:**
```json
{"status": "ok"}
```

**HTTP status:** `200 OK`

### 4.3 Readiness probe

```bash
curl -s http://localhost:8080/api/ready | jq .
```

**Expected (Ollama running):**
```json
{
  "ready": true,
  "checks": [
    {"name": "ollama", "ok": true}
  ]
}
```

**Expected (Ollama NOT running):**
```json
{
  "ready": false,
  "checks": [
    {"name": "ollama", "ok": false, "error": "..."}
  ]
}
```

**HTTP status:** `200` if ready, `503` if not.

### 4.4 Config endpoint

```bash
curl -s http://localhost:8080/api/config | jq .
```

**Expected (no API key set):**
```json
{"auth_required": false}
```

---

## 5. API Endpoint Tests

**All tests below assume the server is running on `localhost:8080`.**

For authenticated mode, set `TFAI_API_KEY=test-key-123` before starting the server and add `-H "Authorization: Bearer test-key-123"` to all curl commands.

### 5.1 Chat — basic question (SSE stream)

```bash
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "what is a terraform module?"}'
```

**Expected:** SSE-formatted stream:
```
data: A Terraform module is...
data: ...reusable infrastructure code...

event: done
data: [DONE]
```

### 5.2 Chat — with workspace context

```bash
# Setup workspace
mkdir -p /tmp/tfai-smoke-ws
echo 'resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }' > /tmp/tfai-smoke-ws/main.tf

curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "what resources are in my workspace?", "workspaceDir": "/tmp/tfai-smoke-ws"}'
```

**Expected:** Response mentions the VPC resource.

### 5.3 Chat — file generation

```bash
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "generate an S3 bucket with versioning", "workspaceDir": "/tmp/tfai-smoke-ws"}'
```

**Expected:** If the LLM responds with the JSON file envelope:
```
data: Created an S3 bucket with versioning...

event: files_written
data: true

event: done
data: [DONE]
```

Check files were written:
```bash
ls /tmp/tfai-smoke-ws/
```

### 5.4 Chat — bad request

```bash
# Missing message field
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{}' -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 400` with `"message is required"`

### 5.5 Workspace listing

```bash
curl -s "http://localhost:8080/api/workspace?dir=/tmp/tfai-smoke-ws" | jq .
```

**Expected:**
```json
{
  "dir": "/tmp/tfai-smoke-ws",
  "files": ["main.tf"],
  "dirs": [],
  "initialized": false,
  "hasState": false,
  "hasLockfile": false
}
```

### 5.6 Workspace — non-existent directory

```bash
curl -s "http://localhost:8080/api/workspace?dir=/tmp/does-not-exist" -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 404` with `"directory not found"`

### 5.7 Workspace — relative path rejected

```bash
curl -s "http://localhost:8080/api/workspace?dir=relative/path" -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 400` with `"dir must be an absolute path"`

### 5.8 Scaffold workspace

```bash
mkdir -p /tmp/tfai-scaffold-test

curl -s -X POST http://localhost:8080/api/workspace/create \
  -H "Content-Type: application/json" \
  -d '{"dir": "/tmp/tfai-scaffold-test", "description": "EKS cluster"}' | jq .
```

**Expected:**
```json
{
  "dir": "/tmp/tfai-scaffold-test",
  "files": ["main.tf", "variables.tf", "outputs.tf", "versions.tf"],
  "prompt": "Create a Terraform workspace for: EKS cluster"
}
```

Verify files exist:
```bash
ls /tmp/tfai-scaffold-test/
# main.tf  outputs.tf  variables.tf  versions.tf
```

### 5.9 Read file

```bash
curl -s "http://localhost:8080/api/file?path=/tmp/tfai-smoke-ws/main.tf&workspaceDir=/tmp/tfai-smoke-ws" | jq .
```

**Expected:**
```json
{
  "path": "/tmp/tfai-smoke-ws/main.tf",
  "content": "resource \"aws_vpc\" \"main\" { cidr_block = \"10.0.0.0/16\" }\n"
}
```

### 5.10 Save file

```bash
curl -s -X PUT http://localhost:8080/api/file \
  -H "Content-Type: application/json" \
  -d '{
    "workspaceDir": "/tmp/tfai-smoke-ws",
    "path": "/tmp/tfai-smoke-ws/outputs.tf",
    "content": "output \"vpc_id\" {\n  value = aws_vpc.main.id\n}\n"
  }' | jq .
```

**Expected:**
```json
{"ok": true}
```

Verify:
```bash
cat /tmp/tfai-smoke-ws/outputs.tf
# output "vpc_id" {
#   value = aws_vpc.main.id
# }
```

### 5.11 File read — path traversal blocked

```bash
curl -s "http://localhost:8080/api/file?path=/etc/passwd&workspaceDir=/tmp/tfai-smoke-ws" -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 403` with `"path is outside the workspace directory"`

### Cleanup

```bash
rm -rf /tmp/tfai-smoke-ws /tmp/tfai-scaffold-test
```

---

## 6. Web UI Smoke Tests

**Requires:** Server running (`./bin/tfai serve`)

1. **Open browser** → `http://localhost:8080`
2. **Verify page loads** — you should see a chat interface and a workspace panel on the left
3. **Set workspace directory** — enter an absolute path to a directory containing `.tf` files
4. **Verify file listing** — workspace panel should show the `.tf` files
5. **Send a chat message** — type a Terraform question and press Enter/Send
6. **Verify streaming** — tokens should appear incrementally (not all at once)
7. **Verify SSE completion** — the input should re-enable after streaming finishes
8. **Click a file** — verify file content displays in the editor panel
9. **Edit and save** — modify content in the editor, click Save, verify the file on disk changed
10. **Generate files via chat** — ask "generate an S3 bucket with versioning" with a workspace set. Verify files appear in the workspace panel.

### If auth is enabled (`TFAI_API_KEY` set)

11. **Refresh the page** — an API key modal should appear
12. **Enter the correct key** — chat and workspace should work
13. **Enter a wrong key** — requests should fail with 401
14. **Open a new incognito window** — modal should appear again (key is in `sessionStorage`)

---

## 7. Authentication Tests

### Start server with auth enabled

```bash
TFAI_API_KEY=test-secret-key ./bin/tfai serve
```

### 7.1 Request without token → 401

```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"hello"}' -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 401` with `"authorization required"`  
**Response header:** `WWW-Authenticate: Bearer realm="tfai"`

### 7.2 Request with wrong token → 401

```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer wrong-key" \
  -d '{"message":"hello"}' -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 401` with `"invalid token"`

### 7.3 Request with correct token → 200

```bash
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-secret-key" \
  -d '{"message":"what is terraform?"}'
```

**Expected:** SSE stream with response.

### 7.4 Health and ready are NOT auth-protected

```bash
curl -s http://localhost:8080/api/health -w "\nHTTP %{http_code}\n"
curl -s http://localhost:8080/api/ready -w "\nHTTP %{http_code}\n"
```

**Expected:** Both return `HTTP 200` without any auth header.

### 7.5 Config endpoint is NOT auth-protected

```bash
curl -s http://localhost:8080/api/config | jq .
```

**Expected:** `{"auth_required": true}`

---

## 8. Rate Limiting Tests

Default: 10 requests/second sustained, burst 20.

```bash
# Fire 25 rapid requests (exceeds burst of 20)
for i in $(seq 1 25); do
  curl -s -o /dev/null -w "%{http_code} " http://localhost:8080/api/health
done
echo ""
```

**Expected:** First ~20 return `200`, remaining return `429`.

Check the `Retry-After` header on a 429 response:

```bash
curl -s -D - -o /dev/null http://localhost:8080/api/health | grep -i retry-after
```

**Note:** `/api/health` is not rate-limited in the current implementation. Use a protected endpoint for this test:

```bash
for i in $(seq 1 25); do
  curl -s -o /dev/null -w "%{http_code} " "http://localhost:8080/api/workspace?dir=/tmp"
done
echo ""
```

---

## 9. Observability Verification

### 9.1 Prometheus metrics

```bash
curl -s http://localhost:8080/metrics | grep tfai_
```

**Expected metrics present:**
```
tfai_chat_requests_total{outcome="ok"} ...
tfai_chat_requests_total{outcome="error"} ...
tfai_chat_requests_total{outcome="timeout"} ...
tfai_chat_duration_seconds_bucket{...}
tfai_chat_active_streams ...
tfai_http_requests_total{...}          # NOTE: currently zeros — tracked in MF-1
tfai_http_duration_seconds_bucket{...} # NOTE: currently zeros — tracked in MF-1
```

After sending a chat request, re-check:
```bash
curl -s http://localhost:8080/metrics | grep 'tfai_chat_requests_total'
```

**Expected:** `outcome="ok"` counter should have incremented by 1.

### 9.2 Structured logs

While the server is running, check stderr output format:

```bash
# JSON format (default for LOG_FORMAT=json)
./bin/tfai serve 2>&1 | head -5
```

**Expected:** Each line is valid JSON with `level`, `msg`, `time` fields.

```bash
# Text format
LOG_FORMAT=text ./bin/tfai serve 2>&1 | head -5
```

**Expected:** Human-readable log lines.

### 9.3 Request ID in logs

Send a request and observe the server logs:

```bash
curl -s http://localhost:8080/api/health > /dev/null
```

**Expected log line contains `request_id`:**
```json
{"level":"INFO","msg":"request","request_id":"a1b2c3d4...","method":"GET","path":"/api/health","status":200,"duration":"0.1ms"}
```

### 9.4 Langfuse tracing (optional)

Requires Langfuse running (`make up` starts it on `localhost:3000`).

1. Set `LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` in `.env`
2. Start the server: `make run`
3. Send a chat request
4. Open Langfuse UI at `http://localhost:3000`
5. Navigate to Traces → verify a trace with the session ID appears
6. Verify the trace contains model generation spans

---

## 10. Docker Compose Stack

### 10.1 Start supporting services only

```bash
make up
```

**Expected:** Qdrant and Langfuse start. Verify:
```bash
docker compose ps
# qdrant, langfuse, langfuse-db should be "running" or "healthy"
```

```bash
# Qdrant health
curl -s http://localhost:6333/healthz
# Expected: empty 200 OK

# Langfuse health
curl -s http://localhost:3000/api/public/health | jq .
# Expected: {"status":"OK"}
```

### 10.2 Start full stack in Docker

```bash
make run-docker
```

**Expected:** All 4 containers start (tfai, qdrant, langfuse, langfuse-db).

```bash
# Verify tfai health inside Docker
curl -s http://localhost:8080/api/health | jq .
```

### 10.3 Stop everything

```bash
make down           # stop containers, keep volumes
make down-volumes   # stop containers AND delete volumes (destructive)
```

---

## 11. Conversation History Tests

### 11.1 Default behaviour (history enabled)

```bash
# Start the server
./bin/tfai serve
```

**Expected log:** `history: store opened path=~/.tfai/history.db`

Send two messages in the same workspace:
```bash
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "my name is Alice", "workspaceDir": "/tmp/tfai-history-test"}'

# Wait for response to complete, then:
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "what is my name?", "workspaceDir": "/tmp/tfai-history-test"}'
```

**Expected:** Second response should reference "Alice" — proving history was injected.

### 11.2 Disable history

```bash
TFAI_HISTORY_DB=disabled ./bin/tfai serve
```

**Expected log:** `history: disabled via TFAI_HISTORY_DB=disabled`

### 11.3 Custom DB path

```bash
TFAI_HISTORY_DB=/tmp/test-history.db ./bin/tfai serve
```

**Expected log:** `history: store opened path=/tmp/test-history.db`

Verify file created:
```bash
ls -la /tmp/test-history.db
```

---

## 12. Security Regression Tests

These tests verify the security controls documented in the README.

### 12.1 Path traversal — file API

```bash
curl -s "http://localhost:8080/api/file?path=/etc/passwd&workspaceDir=/tmp" -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 403` — `"path is outside the workspace directory"`

### 12.2 Path traversal — relative path

```bash
curl -s "http://localhost:8080/api/file?path=/tmp/../etc/passwd&workspaceDir=/tmp" -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 403` — path cleaned by `filepath.Clean` and rejected by `confineToDir`.

### 12.3 File save traversal

```bash
curl -s -X PUT http://localhost:8080/api/file \
  -H "Content-Type: application/json" \
  -d '{"workspaceDir":"/tmp","path":"/etc/evil.tf","content":"pwned"}' -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 403`

### 12.4 Oversized chat body

```bash
# Generate a 2MB payload (exceeds 1 MiB limit)
python3 -c "print('{\"message\":\"' + 'A'*2097152 + '\"}')" | \
  curl -s -X POST http://localhost:8080/api/chat \
    -H "Content-Type: application/json" \
    -d @- -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 400` or `HTTP 413` — request body exceeds `maxChatBodyBytes`.

### 12.5 Chat with relative workspaceDir

```bash
curl -s -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"test","workspaceDir":"relative/path"}' -w "\nHTTP %{http_code}\n"
```

**Expected:** `HTTP 400` — `"workspaceDir must be an absolute path"`

### 12.6 Tool invocation audit log

Start the server and send a chat that triggers a terraform tool call (requires terraform installed and a valid workspace):

```bash
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "run terraform plan in my workspace", "workspaceDir": "/path/to/real/tf/workspace"}'
```

**Expected in server logs:**
```json
{"level":"INFO","msg":"tool: terraform invocation","subcommand":"plan","args":["plan","-no-color"],"workspace":"/path/to/real/tf/workspace"}
```

---

## 13. Graceful Shutdown Test

### 13.1 Clean shutdown

```bash
# Terminal 1: start server
./bin/tfai serve

# Terminal 2: send SIGTERM
kill -TERM $(pgrep tfai)
```

**Expected in Terminal 1 logs:**
- No panic or error
- Process exits with code 0
- If in-flight requests exist, they should drain within 10 seconds

### 13.2 Shutdown during chat stream

```bash
# Terminal 1: start server
./bin/tfai serve

# Terminal 2: start a long chat
curl -s -N -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "write a detailed guide on terraform state management"}'

# Terminal 3: immediately send SIGTERM
kill -TERM $(pgrep tfai)
```

**Expected:** The curl stream may be interrupted (known limitation — see SRE_ASSESSMENT.md SF-6).

---

## 15. RAG Pipeline Smoke Tests

**Requires:** Qdrant running (`make up` or `docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant`) and an embedding-capable model.

### 15.1 Start Qdrant

```bash
# Via docker compose (recommended)
make up

# Or standalone
docker run -d --name qdrant -p 6333:6333 -p 6334:6334 qdrant/qdrant

# Verify Qdrant is healthy
curl -s http://localhost:6333/healthz
# Expected: empty 200 OK
```

### 15.2 Configure embedding environment

```bash
# Ollama (default — pull the embedding model first)
ollama pull nomic-embed-text

export QDRANT_HOST=localhost
export QDRANT_COLLECTION=tfai-docs
# MODEL_PROVIDER=ollama is the default — no extra vars needed

# OpenAI alternative
export MODEL_PROVIDER=openai
export OPENAI_API_KEY=sk-...
# EMBEDDING_MODEL defaults to text-embedding-3-small
# EMBEDDING_DIMENSIONS defaults to 1536
```

### 15.3 Ingest a single document

```bash
./bin/tfai ingest --provider aws \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket
```

**Expected log output:**
```
time=... level=INFO msg="embedder initialised" provider=ollama
time=... level=INFO msg="qdrant store ready" host=localhost port=6334 collection=tfai-docs
time=... level=INFO msg="starting ingestion" sources=1 provider=aws
time=... level=INFO msg="fetching https://registry.terraform.io/..."
time=... level=INFO msg="chunked https://... into N chunks"
time=... level=INFO msg="ingested N chunks from https://..."
time=... level=INFO msg="ingestion complete" sources=1
```

**Verify data landed in Qdrant:**
```bash
# Check collection exists and has points
curl -s http://localhost:6333/collections/tfai-docs | jq '.result.points_count'
# Expected: N > 0
```

### 15.4 Ingest multiple documents

```bash
./bin/tfai ingest --provider aws \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_node_group
```

**Expected:** Two fetch/chunk/ingest cycles logged, point count increases.

### 15.5 Verify RAG context injection via ask

```bash
export QDRANT_HOST=localhost

./bin/tfai ask "what are the required arguments for aws_s3_bucket?"
```

**Expected in server logs (or stderr):**
```
time=... level=INFO msg="rag: retriever ready" host=localhost port=6334 collection=tfai-docs
```

**Expected in response:** Answer should reference specific S3 bucket arguments from the ingested docs (e.g. `bucket`, `force_destroy`) — not generic LLM knowledge.

### 15.6 Verify RAG context injection via serve

```bash
export QDRANT_HOST=localhost
./bin/tfai serve
```

**Expected startup logs:**
```
time=... level=INFO msg="rag: retriever ready" host=localhost port=6334 collection=tfai-docs
```

**Expected readiness probe includes Qdrant:**
```bash
curl -s http://localhost:8080/api/ready | jq .
```
```json
{
  "ready": true,
  "checks": [
    {"name": "ollama", "ok": true},
    {"name": "qdrant",  "ok": true}
  ]
}
```

### 15.7 Verify RAG disabled gracefully when Qdrant absent

```bash
# Unset QDRANT_HOST (or don't set it)
unset QDRANT_HOST
./bin/tfai serve
```

**Expected:** No `rag:` log lines at startup. `ask` and `generate` work normally without RAG context. `/api/ready` shows only the LLM check.

### 15.8 Ingest error path — Qdrant not running

```bash
unset QDRANT_HOST  # or point to a non-running host
QDRANT_HOST=localhost ./bin/tfai ingest --provider aws \
  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket
```

**Expected:** Clear error message, non-zero exit:
```
Error: ingest: failed to connect to Qdrant at localhost:6334: ...
```

### Cleanup

```bash
# Remove the test collection
curl -s -X DELETE http://localhost:6333/collections/tfai-docs
# Or stop Qdrant entirely
docker stop qdrant && docker rm qdrant
```

---

## 14. Known Limitations

These are documented issues that will cause unexpected behaviour during testing. They are tracked in `docs/ROADMAP.md`.

| Issue | ID | Impact on Testing |
|---|---|---|
| `httpRequestsTotal` / `httpDurationSeconds` are always zero | MF-1 | Metrics endpoint shows registered but unincremented counters |
| No body size limit on `/api/workspace/create` and `/api/file` PUT | MF-2 | Oversized payloads on these endpoints won't be rejected |
| `buildWorkspaceContext` has no file/size caps | MF-3 | Very large workspaces may cause slow responses or OOM |
| Shutdown timeout (10s) < Chat timeout (5m) | SF-6 | Active SSE streams are killed during shutdown without error event |
| Bedrock/Gemini embedders not implemented | RAG-5 | `tfai ingest` with `MODEL_PROVIDER=bedrock` or `gemini` returns a clear error |
| HTML stripping is regex-based | RAG-6 | Script/style tag content may appear in chunks; good enough for Terraform Registry docs |

---

## Quick Reference — Test Matrix

Use this checklist after any change:

```
[ ] make gate                          — MUST pass
[ ] ./bin/tfai version                 — prints version
[ ] ./bin/tfai ask "hello"             — LLM responds
[ ] ./bin/tfai serve + curl /api/health — returns 200
[ ] curl /api/ready                    — returns 200 or 503 with check details
[ ] curl POST /api/chat                — SSE stream works
[ ] curl GET /api/workspace?dir=...    — lists files
[ ] curl POST /api/workspace/create    — scaffolds files
[ ] curl GET /api/file                 — reads file
[ ] curl PUT /api/file                 — saves file
[ ] curl GET /api/file (traversal)     — returns 403
[ ] curl GET /metrics                  — Prometheus output present
[ ] Browser http://localhost:8080      — UI loads
[ ] Ctrl+C on server                   — clean shutdown, exit 0

# RAG pipeline (requires Qdrant running)
[ ] tfai ingest --provider aws --url <url>   — chunks ingested, logged
[ ] curl /api/ready (with QDRANT_HOST set)   — qdrant check appears
[ ] tfai ask with QDRANT_HOST set            — RAG context injected (check logs)
[ ] tfai serve with QDRANT_HOST set          — "rag: retriever ready" in startup logs
```
