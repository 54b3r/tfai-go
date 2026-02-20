# Security & SRE Rules — tfai-go
# These rules apply to all sessions in this project workspace.
# Security is a first-class citizen throughout the SDLC — not an afterthought.

---

## Filesystem Security

- **Path confinement is mandatory** for every filesystem operation that accepts
  user-supplied or LLM-supplied paths. Use `confineToDir(root, target)` from
  `internal/server/workspace.go` or an equivalent separator-aware prefix check:
  ```go
  strings.HasPrefix(target+string(filepath.Separator), root+string(filepath.Separator))
  ```
  A plain `strings.HasPrefix(target, root)` is NEVER sufficient — it allows
  `/tmp/foo` to match `/tmp/foobar`.

- **Never call `os.MkdirAll` on user-controlled or LLM-controlled paths** unless
  the path has already been confined to a validated workspace root.

- **Never call `os.WriteFile` or `os.ReadFile` on unvalidated paths.** Every file
  operation must pass through `confineToDir` first.

- **The server does not create directories.** Workspace directories must be
  pre-existing on the filesystem. `POST /api/workspace/create` scaffolds files
  into an existing directory only — it never creates the directory itself.

- **File permissions:** files written by the server use `0o644`, directories
  created by `applyFiles` use `0o755`. Never use `0o777`.

---

## HTTP Server Security

- **Request body size limits** must be applied on every POST/PUT handler using
  `http.MaxBytesReader` before decoding. Current limit: `1 MiB` for `/api/chat`.
  Add equivalent limits to any new endpoints that accept a body.

- **CORS** must not use `Access-Control-Allow-Origin: *`. This server is
  localhost-only. Restrict to `http://127.0.0.1:<port>` and
  `http://localhost:<port>` only. Reject requests from other origins silently
  (do not set the header).

- **Input validation** on every handler:
  - All path parameters must be absolute (`filepath.IsAbs`)
  - All path parameters must be confined to a declared workspace root
  - Empty required fields must return `400 Bad Request` before any I/O

- **Error messages** returned to the client must not leak internal filesystem
  paths, stack traces, or system information. Log full detail server-side,
  return a generic message to the client.

- **Static file serving** must use an embedded filesystem (`embed.FS`) or an
  absolute path — never `http.Dir("relative/path")` which breaks when the
  binary is run from a different working directory.

---

## Secret Handling

- **No secrets in source code, ever.** API keys, tokens, passwords must only
  come from environment variables.

- **`.env` files are gitignored.** Only `.env.example` with placeholder values
  may be committed. Verify with `git status` before every commit.

- **Before every PR**, run a secret scan:
  ```bash
  git diff origin/main... | grep -iE '(api_key|secret|password|token|pk-lf|sk-lf|Bearer)\s*[:=]\s*\S+'
  ```
  If any real values appear, abort and rotate the secret immediately.

- **Langfuse keys, Azure API keys, and model credentials** must never appear in
  logs, error messages, or HTTP responses.

---

## Dependency Security

- **Pin dependencies** in `go.mod`. Never use `latest` or floating version
  constraints in production code.

- **Run `go mod tidy`** after every dependency change to remove unused modules.

- **Audit new dependencies** before adding them:
  - Check for known CVEs: `govulncheck ./...`
  - Prefer packages with active maintenance and a clear security policy
  - Minimise the dependency surface — prefer stdlib where feasible

- **`govulncheck`** should be added to the `make gate` target once installed:
  ```makefile
  govulncheck ./...
  ```

---

## LLM / Agent Security

- **LLM output is untrusted input.** Treat every field in `TerraformAgentOutput`
  (file paths, content) as adversarial. Always run through `confineToDir` before
  writing to disk.

- **Prompt injection awareness.** Workspace file contents are injected into the
  system prompt via `buildWorkspaceContext`. If a `.tf` file contains adversarial
  instructions, they will reach the LLM. Mitigations:
  - Only inject `.tf` and `.tfvars` files (already enforced)
  - Cap total injected context size (TODO: add a byte limit to `buildWorkspaceContext`)
  - Never inject files from outside the declared workspace root

- **RAG content is untrusted.** Documents retrieved from Qdrant are injected into
  the system prompt. Ensure the Qdrant instance is not publicly accessible and
  only contains content ingested by the operator.

- **Tool surface area.** Every Eino tool registered with the agent can be invoked
  by the LLM autonomously. Only register tools that are safe to call without
  human confirmation. Tools that execute shell commands or make network requests
  require explicit user confirmation before being added.

---

## Observability & SRE

- **Structured logging** for all security-relevant events:
  - File read/write: log path and outcome (already in place)
  - Path confinement rejections: log at WARN with the attempted path
  - Request body limit exceeded: log at WARN with remote addr
  - Authentication failures (future): log at ERROR

- **Never swallow errors silently.** `_ = err` is forbidden except in explicit
  defer cleanup. RAG errors must be logged even if non-fatal.

- **Health endpoint** (`GET /api/health`) must remain unauthenticated and must
  not expose internal state, version details, or dependency health in the
  response body (keep it `{"status":"ok"}`).

- **Graceful shutdown** must be implemented for all long-running processes.
  The server already uses `context` cancellation + `httpServer.Shutdown`.
  Maintain this pattern for any new background workers.

- **Timeouts** must be set on all outbound HTTP clients and all server handlers.
  Current values: `ReadTimeout: 30s`, `WriteTimeout: 5m` (SSE streaming).
  Do not remove or increase these without justification.

---

## SDLC Gates

These checks must pass before any commit or PR. They are encoded in `make gate`:

1. `go build ./...` — zero errors
2. `go vet ./...` — zero warnings
3. `go test -race -count=1 ./...` — all tests pass
4. Binary smoke test — `version`, `--help`, `serve --help` all exit 0
5. Secret scan — no credentials in the diff
6. (Planned) `govulncheck ./...` — zero known CVEs

**A failing gate blocks the commit.** There are no exceptions.

---

## Threat Model (current scope: local-only tool)

The server binds to `127.0.0.1` by default and is intended for single-user
local use. The primary threats are:

| Threat | Mitigation |
|---|---|
| Path traversal via LLM output | `confineToDir` on all file writes |
| Path traversal via API params | `confineToDir` on all file API calls |
| Arbitrary directory creation | `MkdirAll` only within confined workspace |
| Oversized request DoS | `http.MaxBytesReader` on all POST/PUT |
| Secret leakage via logs | Secrets only from env vars, never logged |
| Prompt injection via workspace files | Only `.tf`/`.tfvars` injected, size cap TODO |
| SSRF via model base URL | URL comes from operator env vars only |

When the server is exposed beyond localhost (future), add:
- Authentication (at minimum a static bearer token from env)
- TLS termination
- Rate limiting per client IP
- Audit log of all file operations
