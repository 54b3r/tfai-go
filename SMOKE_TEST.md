# Smoke Test Runbook

Manual validation steps for key features. Run the relevant section after changes to the areas noted.

---

## Part 1 — LLM File Extraction

Run after any change to `Query()`, `parseAgentOutput()`, `applyFiles()`, the system prompt, or the `sendMessage` frontend function.

## Prerequisites

- Model provider env vars set in your shell (e.g. `MODEL_PROVIDER=azure` + Azure credentials)
- Server built and running: `make gate && ./bin/tfai serve`
- A writable temp directory to use as the workspace

## Steps

### 1. Create a workspace directory

```bash
mkdir -p /tmp/tfai-smoke
```

### 2. Start the server

```bash
TFAI_API_KEY=smoke-test-key go run ./cmd/tfai serve
# Expected log lines:
#   level=INFO msg="auth enabled" api_key_set=true
#   level=INFO msg="server listening" addr=http://127.0.0.1:8080
```

### 3. Open the UI

Navigate to http://127.0.0.1:8080 in your browser.

### 4. Set the workspace directory

In the **Workspace** sidebar input, enter:
```
/tmp/tfai-smoke
```
Click **→** to load it (should show empty or scaffold files).

### 5. Send a generate prompt

In the chat input, send:
```
Generate a simple S3 bucket with versioning enabled
```

### 6. Verify expected behaviour

| # | What to check | Expected |
|---|---------------|----------|
| 1 | Chat bubble content | Shows a human-readable **summary sentence**, NOT raw JSON |
| 2 | File tree (sidebar) | Refreshes automatically without a manual reload |
| 3 | Files on disk | `ls /tmp/tfai-smoke` shows `.tf` files (e.g. `main.tf`, `variables.tf`) |
| 4 | File contents | `cat /tmp/tfai-smoke/main.tf` contains valid HCL, not JSON |

### 7. Verify module path handling (optional but recommended)

Send:
```
Generate a reusable S3 module with a root caller
```

Expected: `ls /tmp/tfai-smoke/modules/s3/` shows `main.tf`, `variables.tf`, `outputs.tf`.

### 8. Cleanup

```bash
rm -rf /tmp/tfai-smoke
```

## Failure modes to watch for

- **Raw JSON appears in chat bubble** — `parseAgentOutput` failed or LLM did not follow the schema; check system prompt and LLM response in server logs
- **File tree does not refresh** — `event: files_written` SSE frame not received; check browser DevTools → Network → `/api/chat` response stream
- **No files on disk** — `applyFiles` returned an error; check server stderr for `agent: Query: failed to apply files`
- **Summary is empty** — LLM returned JSON with an empty `summary` field; the system prompt may need reinforcement

---

## Part 4 — Authentication & Rate Limiting

Run after any change to `internal/server/auth.go`, `internal/server/ratelimit.go`, or the middleware wiring in `server.go`.

### Prerequisites

- Server built: `make gate`
- Server running with an API key set:
  ```bash
  TFAI_API_KEY=smoke-test-key MODEL_PROVIDER=ollama OLLAMA_MODEL=llama3 \
    ./bin/tfai serve --port 8099
  ```

### Steps

#### 1. Unprotected routes — no auth required

```bash
curl -s http://127.0.0.1:8099/api/health
# Expected: 200 {"status":"ok"}

curl -s http://127.0.0.1:8099/api/ready
# Expected: 200 or 503 depending on LLM/Qdrant state — never 401
```

#### 2. Protected route — missing token

```bash
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8099/api/workspace?dir=/tmp
# Expected: 401
```

Server log should show:
```
level=WARN msg="auth: missing Authorization header" path=/api/workspace
```

#### 3. Protected route — wrong token

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer wrong-key" \
  http://127.0.0.1:8099/api/workspace?dir=/tmp
# Expected: 401
```

Server log should show:
```
level=WARN msg="auth: invalid token" path=/api/workspace token_present=true
```

Verify the actual key value does **not** appear in the log.

#### 4. Protected route — correct token

```bash
curl -s -H "Authorization: Bearer smoke-test-key" \
  http://127.0.0.1:8099/api/workspace?dir=/tmp
# Expected: 200 with JSON workspace response
```

#### 5. Auth disabled mode (dev)

Restart without `TFAI_API_KEY`:
```bash
MODEL_PROVIDER=ollama OLLAMA_MODEL=llama3 ./bin/tfai serve --port 8099
```

Expected startup log:
```
level=WARN msg="auth disabled: TFAI_API_KEY not set — all API routes are unauthenticated"
```

All routes should respond without an Authorization header.

#### 6. Rate limiting

```bash
for i in $(seq 1 30); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -H "Authorization: Bearer smoke-test-key" \
    http://127.0.0.1:8099/api/workspace?dir=/tmp
done
```

Expected: first ~20 responses are `200`, subsequent responses include at least one `429`.
Server log should show:
```
level=WARN msg="rate limit exceeded" ip=127.0.0.1 path=/api/workspace
```

### Failure modes

- **401 on `/api/health` or `/api/ready`** — auth middleware incorrectly applied to exempt routes; check `server.go` mux wiring
- **200 on protected route without token** — `authMiddleware` not wired; check `protected()` closure in `server.go`
- **API key value appears in logs** — security regression in `auth.go`; only `token_present:true/false` should be logged
- **No 429 after burst** — rate limiter not wired; check `rl.middleware` wrapping in `server.go`

---

## Part 2 — Langfuse Tracing

Run after any change to `internal/tracing/`, `serve.go` tracing wiring, or the Langfuse callback integration.

### Prerequisites

- `make up` — Langfuse and Qdrant running in Docker
- Langfuse account + project created at http://localhost:3000
- API keys generated: **Settings → API Keys → Create new API key**

### Steps

#### 1. Export Langfuse credentials

```bash
export LANGFUSE_HOST=http://localhost:3000
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
```

#### 2. Start the server with tracing enabled

```bash
MODEL_PROVIDER=azure ./bin/tfai serve
```

Expected startup log:
```
serve: langfuse tracing enabled
```

If you see `serve: langfuse tracing disabled` — the env vars are not exported (see `export` above).

#### 3. Send a chat message

Open http://127.0.0.1:8080, set workspace to `/tmp/tfai-smoke`, and send:
```
Generate a simple S3 bucket with versioning enabled
```

#### 4. Verify traces in Langfuse UI

1. Open http://localhost:3000 → your project → **Traces**
2. A new trace should appear for the request
3. Expand it — expect to see:
   - LLM call node with model name, token counts, and latency
   - Tool call nodes (if any tools were invoked)
   - RAG retrieval node (if RAG is configured)

### Failure modes

- **No traces appear** — check that `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` are exported (not just set), and that the startup log shows `tracing enabled`
- **`serve: langfuse tracing disabled`** — env vars not exported to child process; use `export` or inline prefix: `LANGFUSE_PUBLIC_KEY=pk-... ./bin/tfai serve`
- **Traces appear but are empty** — Langfuse callback registered but flush not called; ensure `defer flush()` is in place in `serve.go`

---

## Part 3 — File Editor & Workspace Security

Run after any change to `handleFileRead`, `handleFileSave`, `handleWorkspaceCreate`, `confineToDir`, or the file editor UI.

### Prerequisites

- Server running: `./bin/tfai serve`
- A workspace directory with `.tf` files loaded in the UI

### Steps

#### 1. Open a file from the sidebar

1. Load a workspace that contains `.tf` files
2. Click any file in the sidebar tree
3. Expected: file content appears in the editor panel on the right

#### 2. Edit and save a file

1. Modify the file content in the editor
2. Click **Save**
3. Expected: unsaved indicator disappears; `cat <workspace>/<file>` on disk shows the updated content

#### 3. Discuss file content in chat

1. With a file open in the editor, click **Discuss**
2. Expected: a chat message is pre-filled referencing the file; agent responds with context-aware advice

#### 4. Workspace scaffolding — existing directory

```bash
mkdir -p /tmp/tfai-existing
```

1. In the **Workspace** sidebar, enter `/tmp/tfai-existing` and click **Scaffold starter files**
2. Expected: `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf` appear in the directory and the file tree refreshes

#### 5. Workspace scaffolding — non-existent directory (security check)

1. Enter a path that does not exist, e.g. `/tmp/tfai-does-not-exist`
2. Click **Scaffold starter files**
3. Expected: error response — the server returns `400 Bad Request`; no directory is created on disk

```bash
ls /tmp/tfai-does-not-exist  # must not exist
```

#### 6. Path traversal rejection (security check)

```bash
curl -s -H "Authorization: Bearer smoke-test-key" \
  "http://127.0.0.1:8080/api/file?path=../../etc/passwd&workspaceDir=/tmp/tfai-existing"
# Expected: 403 Forbidden
```

### Failure modes

- **File does not save** — check server logs for `server: file save error`; verify `workspaceDir` is sent in the PUT body (DevTools → Network)
- **Scaffold creates directory** — regression in `handleWorkspaceCreate`; `os.MkdirAll` must not be called on the workspace root
- **Path traversal not rejected** — `confineToDir` not applied; check `handleFileRead` and `handleFileSave` in `workspace.go`
