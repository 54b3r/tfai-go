# Smoke Test — LLM File Extraction (feat/llm-file-extraction)

Run these steps after any change to `Query()`, `parseAgentOutput()`, `applyFiles()`,
the system prompt, or the `sendMessage` frontend function.

## Prerequisites

- `OPENAI_API_KEY` (or equivalent provider env var) set in your shell
- Server built and running: `make run` or `go run ./cmd/tfai serve`
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
