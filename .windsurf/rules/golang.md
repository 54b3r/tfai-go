---
description: Go coding standards and best practices for the tfai-go project
---

# Go Development Rules — tfai-go

Adapted from [github/awesome-copilot go.instructions.md](https://github.com/github/awesome-copilot/blob/main/instructions/go.instructions.md),
extended with AI agent, Eino framework, and LLM provider conventions.

---

## General

- Write simple, clear, and idiomatic Go code. Favor clarity over cleverness.
- Follow the principle of least surprise.
- Keep the happy path left-aligned — return early to reduce nesting.
- Prefer `if condition { return }` over if-else chains.
- Make the zero value useful.
- Write self-documenting code with clear, descriptive names.
- Document ALL exported types, functions, methods, constants, and packages — no exceptions.
- Use Go modules for dependency management (`go.mod` / `go.sum`).
- Prefer standard library solutions over custom implementations when the functionality exists.
- Never use emoji in code, comments, or documentation.

---

## Naming Conventions

### Packages
- Lowercase, single-word names. No underscores, hyphens, or mixedCaps.
- Name packages by what they provide, not what they contain.
- Avoid generic names: `util`, `common`, `base`, `helpers`.
- Package names are singular, not plural.
- **CRITICAL**: Each `.go` file has exactly ONE `package` declaration at the top.
  Never duplicate it. When editing an existing file, preserve the existing declaration.

### Variables and Functions
- Use `mixedCaps` (unexported) or `MixedCaps` (exported) — never underscores.
- Keep names short but descriptive. Single-letter variables only for tight loop scopes.
- Exported names start with a capital letter; unexported with lowercase.
- Avoid stuttering: prefer `http.Server` over `http.HTTPServer`.

### Interfaces
- Use the `-er` suffix where possible (`Reconciler`, `Reader`, `Watcher`).
- Single-method interfaces are named after the method.
- Keep interfaces small and focused (1–3 methods is ideal).
- Define interfaces close to where they are used, not where they are implemented.

### Constants
- Exported: `MixedCaps`. Unexported: `mixedCaps`.
- Group related constants in `const` blocks.
- Use typed constants for better type safety.

---

## Code Style and Formatting

- Always run `gofmt` before committing. CI must enforce this.
- Use `goimports` to manage import grouping automatically.
- Import groups (in order, separated by blank lines):
  1. Standard library
  2. Third-party packages
  3. Internal / project packages
- Add blank lines to separate logical groups within a function.
- Keep functions focused — if a function needs a comment to explain what each section does, it should probably be split.

---

## Comments and Documentation

- **Every exported symbol must have a doc comment.** No exceptions.
- Doc comments start with the name of the symbol: `// WebAppReconciler reconciles a WebApp resource.`
- Package comments start with `// Package <name>`.
- Use `//` line comments for all inline and block documentation.
- Document **why**, not **what** — unless the what is genuinely complex.
- Comments are complete English sentences. No trailing punctuation on error messages.
- Inline comments explain non-obvious logic, business rules, or operator-specific behavior.
- Every struct field that is exported must have a comment explaining its purpose.

### Agent and Tool Comment Rules
- Every Eino `tool.BaseTool` implementation must have a doc comment explaining what the tool
  does, what inputs it expects, and what it returns to the agent.
- Every provider backend constructor (`newOllama`, `newAzure`, etc.) must document which env
  vars it requires and what happens when they are missing.
- Every `Retriever` or `VectorStore` method must document its error behaviour on empty results.

---

## Error Handling

- Check errors immediately after every function call.
- Never ignore errors with `_` unless you explicitly document why.
- Wrap errors with context: `fmt.Errorf("reconciling deployment: %w", err)`.
- Use `errors.New` for static errors, `fmt.Errorf` for dynamic ones.
- Export sentinel errors for cases callers need to check: `var ErrNotFound = errors.New(...)`.
- Use `errors.Is` and `errors.As` for error inspection — never string matching.
- Error messages: lowercase, no trailing punctuation.
- Place error return as the last return value.
- Name error variables `err` (or `err<Thing>` when multiple errors are in scope).
- **Never log and return an error** — choose one. In operators, return the error and let
  `controller-runtime` handle requeue logging.

---

## Structs and Types

- Define types to add meaning and type safety — avoid stringly-typed APIs.
- Use struct tags for all JSON, YAML, and Kubernetes API fields.
- Every struct must have a doc comment explaining its purpose and role.
- Every exported struct field must have a doc comment.
- Use pointer receivers when the method modifies the receiver or the struct is large.
- Use value receivers for small, immutable structs.
- Be consistent within a type's method set — don't mix pointer and value receivers.
- Prefer `any` over `interface{}` (Go 1.18+). Avoid unconstrained types where possible.

### AI Agent and Provider Type Rules
- `Config` structs for providers must document every field with the env var that populates it.
- Optional fields that have defaults must document the default value in the field comment.
- Interface types (`VectorStore`, `Retriever`, `Runner`) must document thread-safety guarantees.
- Use pointer types (`*int`, `*float32`) for optional numeric fields passed to external SDKs
  so that zero values are distinguishable from unset.

---

## Architecture and Project Structure

- Provider backends live in `internal/provider/` — one file per backend, one file for the interface, one for the factory.
- RAG abstractions live in `internal/rag/` — interfaces in `interface.go`, implementations in named files (`qdrant.go`).
- Terraform tools live in `internal/tools/` — one file per tool, `runner.go` for the CLI abstraction.
- Agent wiring lives in `internal/agent/` — the agent must not import CLI or server packages.
- HTTP server lives in `internal/server/` — must not import CLI packages.
- CLI commands live in `cmd/tfai/commands/` — may import agent, provider, tools, server.
- `cmd/tfai/main.go` is the entrypoint only — no business logic.
- Avoid circular dependencies — the dependency graph flows: cmd → internal/agent → internal/provider, internal/tools, internal/rag.
- Use `go mod tidy` regularly to clean up unused dependencies.
- Keep dependencies minimal — every new dependency is a maintenance and security surface.

---

## Concurrency

- Never start a goroutine without knowing how it will stop.
- Use `context.Context` for cancellation — pass it as the first argument to every function
  that does I/O or long-running work.
- Prefer channels for communication, mutexes for state protection.
- Always use `defer` for cleanup (closing files, releasing locks, removing finalizers).
- Never modify maps concurrently without synchronization.

---

## Testing

- Test files use `_test.go` suffix, placed next to the code they test.
- Use table-driven tests for multiple scenarios.
- Name tests: `Test_FunctionName_Scenario` (e.g., `Test_NewFromEnv_MissingAPIKey`).
- Use `t.Run` subtests for organization.
- Test both success and error paths.
- Mark helper functions with `t.Helper()`.
- Use `t.Cleanup()` for resource teardown.
- Provider tests must use a fake/stub `ChatModel` — never make real API calls in unit tests.
- Tool tests must use a fake `Runner` implementation — never invoke a real terraform binary.
- RAG tests must use an in-memory `VectorStore` stub — never require a live Qdrant instance.
- Integration tests that require external services (Qdrant, Ollama) must be in a separate
  `_integration_test.go` file and gated with `//go:build integration`.

---

## Common Pitfalls — Never Do These

- Ignoring errors (even in tests).
- Goroutine leaks — always ensure goroutines can exit.
- Not using `defer` for cleanup.
- Modifying maps concurrently.
- Using global variables unnecessarily — use dependency injection via struct fields.
- Forgetting to close resources (files, HTTP response bodies, DB connections, SSE streams).
- Not understanding nil interface vs nil pointer (a nil `*MyType` stored in an interface is not nil).
- Duplicate `package` declarations — compile error, always check existing files first.
- In providers: passing a zero `int` or `float32` to an SDK that treats zero as "unset" — use pointers.
- In tools: writing to the filesystem without checking for path traversal (user-supplied paths must be validated).
- In the agent: importing `cmd/` packages — the agent must remain CLI-agnostic.
- In the server: blocking the request goroutine on a long LLM stream without respecting `ctx.Done()`.

---

## Pre-Commit Verification Gate

**NEVER commit code that has not passed all of the following checks in order.**
This applies to Cascade (AI) and human contributors equally.

### Required before every commit

1. **Build passes cleanly**
   ```bash
   go build ./...
   ```
   Zero errors, zero warnings.

2. **Vet passes**
   ```bash
   go vet ./...
   ```

3. **Binary smoke test — CLI help**
   ```bash
   go build -o bin/tfai ./cmd/tfai
   ./bin/tfai --help
   ./bin/tfai ask --help
   ./bin/tfai generate --help
   ./bin/tfai diagnose --help
   ./bin/tfai serve --help
   ./bin/tfai ingest --help
   ```
   Every subcommand must be listed and return exit code 0.

4. **Provider validation smoke test** (no model required)
   ```bash
   MODEL_PROVIDER=openai ./bin/tfai ask "test" 2>&1 | grep -q "MODEL_API_KEY"
   MODEL_PROVIDER=bogus  ./bin/tfai ask "test" 2>&1 | grep -q "unknown backend"
   ```
   Both must produce the expected error messages, not panics.

5. **Unit tests pass**
   ```bash
   go test -race -count=1 ./...
   ```
   (Skip if no `_test.go` files exist yet — but add tests before the next feature.)

### Required before merging to main

- All of the above.
- `gofmt -l .` produces no output (no unformatted files).
- `go mod tidy` produces no diff in `go.mod` / `go.sum`.
- At least one real provider has been smoke-tested end-to-end (Ollama locally, or Azure/OpenAI with real credentials).
- PR description documents what was tested and how.
