# tfai-go Workflow Rules

> These rules are project-specific. Workspace-wide rules (source verification,
> Go code quality, pre-commit gate, SRE standards, security) live in:
> `/Users/marvin.matos_1/go/.windsurfrules`
>
> Go coding standards for this project live in:
> `.windsurf/rules/golang.md`

---

## PR and Release Discipline

- **A PR must be merged AND tagged before starting the next task.**
  Never begin a new feature branch while a previous PR is open and unmerged,
  unless the user explicitly says otherwise.
- **Tag immediately after merging to main.** Every merged PR gets a semver
  annotated tag pushed to origin before the next branch is created.
- **Sequence per task:**
  1. `git checkout main && git pull` — confirm merge is present locally
  2. `git tag -a vX.Y.Z -m "<summary>"` and `git push origin vX.Y.Z`
  3. `git checkout -b <type>/<description>` for the next task

## Branch Naming

- `feat/<short-description>`
- `fix/<short-description>`
- `chore/<short-description>`
- `test/<short-description>`
- `docs/<short-description>`

## Pre-Commit Gate

Run `make gate` before every commit. It must exit 0. Never commit a broken state.
`make gate` runs: build → vet → lint → test -race → binary smoke.

## Smoke Test Before Commit

For any change that touches the HTTP server, middleware, or provider wiring,
run a manual smoke test against the running binary before committing:

```bash
./bin/tfai serve --port 8099 > /tmp/tfai-smoke.log 2>&1 &
sleep 2
curl -s http://127.0.0.1:8099/api/health
curl -s "http://127.0.0.1:8099/api/workspace?dir=/tmp"
kill %1
cat /tmp/tfai-smoke.log
```

Verify:
- `{"status":"ok"}` returned from `/api/health`
- Structured log lines present with `request_id`, `method`, `path`, `status`, `duration`
- No panics or unhandled errors in the log

---

## SRE Validation — tfai-go Specifics

These extend the workspace-wide SRE standards for this project:

- **Health endpoint** (`GET /api/health`): liveness only — must always return 200
  while the process is running, regardless of LLM provider state.
- **Readiness endpoint** (`GET /api/ready`): must probe LLM provider reachability
  and Qdrant (if configured). Return 503 if any required dependency is unreachable.
- **Timeouts**: all outbound LLM and Qdrant calls must have a context deadline.
  Never pass a background context to a provider call — always derive from the request context.
- **Body size limits**: `http.MaxBytesReader` must be applied on all POST/PUT handlers
  before any decoding. Current limit: 1 MiB for chat, 512 KiB for file save.
- **Graceful shutdown**: the server drains in-flight SSE streams before exit.
  `ShutdownTimeout` defaults to 10s — do not reduce it.
- **Rate limiting** (GH #17): must be applied at the mux level before handlers run,
  not inside individual handlers.

---

## Security — tfai-go Specifics

These extend the workspace-wide security rules for this project:

- **Path traversal**: `confineToDir(root, target)` in `workspace.go` is the canonical
  guard. All file read/write handlers must call it. Never bypass it.
- **Workspace dir validation**: `workspaceDir` must be an absolute path. Validate with
  `filepath.IsAbs(filepath.Clean(path))` before any filesystem operation.
- **LLM prompt injection**: never interpolate raw user input directly into system prompts.
  User messages are always passed as `schema.HumanMessage`, never concatenated into the
  system prompt string.
- **API keys in logs**: provider config validation may log which keys are present/absent
  (`OPENAI_API_KEY set: true`) but must never log the key value itself.
- **CORS**: the server restricts `Access-Control-Allow-Origin` to the configured
  localhost origin only. Never set `*` as the allowed origin.
- **Auth** (GH #19): once the auth middleware is implemented, it must be wired into the
  mux before all `/api/*` handlers except `/api/health` and `/api/ready`.
