# Smoke Test Runbook

Manual validation steps for key features. Run the relevant section after changes to the areas noted.

---

## Part 1 — LLM File Extraction

Run after any change to `Query()`, `parseAgentOutput()`, `applyFiles()`, the system prompt, or the `sendMessage` frontend function.

## Prerequisites

- Model provider env vars set in your shell (e.g. `MODEL_PROVIDER=azure` + Azure credentials)
- Server built and running: `make build && ./bin/tfai serve`
- A writable temp directory to use as the workspace

## Steps

### 1. Create a workspace directory

```bash
mkdir -p /tmp/tfai-smoke
```

### 2. Start the server

```bash
go run ./cmd/tfai serve
# Expected: "tfai server listening on http://127.0.0.1:8080"
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
