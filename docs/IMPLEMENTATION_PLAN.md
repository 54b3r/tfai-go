# TF-AI: System Prompt v2 + RAG Pipeline + Evaluation Framework

> Implementation plan for upgrading TF-AI from "generates Terraform" to
> "generates production-grade, auditable, reusable Terraform modules with a
> Platform Engineering mindset."

## Problem Statement

The current system prompt produces structurally valid but shallow Terraform.
Generated EKS modules are missing ~50% of what a Senior Platform Engineer
would include: encryption, IRSA, managed add-ons, tagging, explicit security
groups, IMDSv2, and meaningful comments beyond resource names.

Root causes:
1. **System prompt** — tells the model to be an "expert" but never defines
   what that means operationally (no security baseline, no module design
   philosophy, no self-audit step).
2. **No RAG** — the model generates from training data, not from live
   provider docs. Resource arguments, Atmos conventions, and CIS benchmarks
   are absent from context.
3. **No evaluation** — no structured way to measure whether a change to the
   prompt or RAG corpus actually improves output quality.

## Architecture: Three Layers of Intelligence

```
┌──────────────────────────────────────────────────────────┐
│  Layer 1: System Prompt (The Philosophy)                 │
│  - Static, changes quarterly                             │
│  - Defines mindset, standards, self-audit checklist      │
│  - Stored in code (agent.go or embedded .md files)       │
├──────────────────────────────────────────────────────────┤
│  Layer 2: RAG Corpus (The Knowledge)                     │
│  - Dynamic, changes when providers ship new resources    │
│  - Provider schemas, CIS benchmarks, framework patterns  │
│  - Stored in Qdrant, injected per-query via retriever    │
├──────────────────────────────────────────────────────────┤
│  Layer 3: Prompt Profiles (The Optimization)             │
│  - Per-model tuning (reasoning vs chat vs compact)       │
│  - Instruction style, chain-of-thought, output format    │
│  - Selected by MODEL_PROVIDER + deployment name          │
└──────────────────────────────────────────────────────────┘
```

## Implementation Phases

### Phase 1: System Prompt v2 (Branch: `feat/system-prompt-v2`)

**Goal:** Rewrite the system prompt so the model thinks like a Senior Platform
Engineer before generating a single line of HCL.

#### Components to Build

1. **Persona definition** — Replace "Terraform expert" with a Platform
   Engineer / SRE persona that:
   - Thinks in terms of security, networking, IAM, observability, lifecycle
   - Designs for reusability (modules with clean interfaces)
   - Applies sane defaults and secure-by-default patterns
   - Comments *why*, not just *what*

2. **Production baseline definition** — Explicitly define what "production-grade"
   means. The model must address all applicable items:
   - Encryption at rest (KMS/CMEK) and in transit (TLS/private endpoints)
   - Least-privilege IAM with explicit trust policies
   - Tagging on every resource (cost allocation, compliance, automation)
   - Logging and audit trails enabled
   - Network isolation (private subnets, explicit security groups)
   - Provider-specific hardening (IMDSv2, IRSA, managed add-ons, etc.)

3. **Module design philosophy** — Instruct the model on module interface quality:
   - Every variable: `description`, `type`, `default` where sensible, `validation` where applicable
   - Every output: `description`, and include outputs downstream consumers need
   - Every resource: comment block explaining purpose and any non-obvious decisions
   - Section headers grouping related resources (IAM, Networking, Compute, etc.)
   - No dead variables — every declared variable must be referenced

4. **Self-audit step** — Before returning generated code, the model must
   mentally verify:
   - "Did I address encryption, IAM, networking, observability, and tagging?"
   - "Are all variables used? Are all outputs useful to a caller?"
   - "Would this pass a code review from a Senior Platform Engineer?"

5. **JSON output contract** — Keep the existing `{ "files": [...], "summary": "..." }`
   structure but add a `checklist` field:
   ```json
   {
     "files": [...],
     "summary": "...",
     "checklist": {
       "encryption": true,
       "iam_least_privilege": true,
       "tagging": true,
       "logging": true,
       "network_isolation": true,
       "all_variables_used": true
     }
   }
   ```

#### Files to Modify
- `internal/agent/agent.go` — rewrite `systemPrompt` const
- `internal/agent/agent.go` — parse and log checklist from generate output
- `cmd/tfai/commands/generate.go` — no change needed if system prompt is sufficient

#### Validation
- `make fs/eks-v2-test` → generate EKS module → manually compare against Phase 1 baseline
- `make fs/s3-v2-test` → generate S3 module → verify encryption, versioning, logging present
- `make gate` — all tests pass

---

### Phase 2: RAG Pipeline (Branch: `feat/rag-pipeline`)

**Goal:** Populate Qdrant with provider-specific docs so the model has grounded
knowledge of resource schemas, arguments, and best practices.

**Depends on:** Phase 1 (system prompt v2 must be in place so the model knows
*how* to use the RAG context effectively)

#### Components to Build

1. **Ingestion sources** — Define the minimum viable corpus:
   - AWS: `aws_eks_cluster`, `aws_eks_node_group`, `aws_eks_addon`,
     `aws_iam_openid_connect_provider`, `aws_s3_bucket`, `aws_kms_key`
   - Azure: `azurerm_kubernetes_cluster`, `azurerm_virtual_network`,
     `azurerm_key_vault`
   - GCP: `google_container_cluster`, `google_compute_network`,
     `google_kms_crypto_key`
   - Frameworks: Atmos stack structure, Terragrunt patterns (if applicable)

2. **Ingestion pipeline validation** — Verify docs are chunked, embedded, and
   retrievable:
   - `make ingest-aws` → confirm vector count in Qdrant
   - Query Qdrant directly for "EKS encryption_config" → verify relevant chunk returned
   - Run `tfai ask "what arguments does aws_eks_cluster support?"` → verify RAG context appears

3. **Retriever tuning** — Adjust `RAGTopK` and chunk size:
   - Default `topK=5` may be too few for complex resources
   - Chunk size must balance precision (small chunks) vs context (large chunks)

#### Files to Modify
- `Makefile` — expand `ingest-aws`, `ingest-azure`, `ingest-gcp` with more URLs
- `internal/rag/` — potential chunk size tuning
- `internal/agent/agent.go` — potential `RAGTopK` adjustment

#### Validation
- Generate EKS module with RAG → compare against Phase 1 (no-RAG) output
- Verify `encryption_config`, `aws_eks_addon`, OIDC provider are now present
- `make gate`

---

### Phase 3: Evaluation Framework (Branch: `feat/eval-framework`)

**Goal:** Structured, repeatable way to measure generate output quality so
every change to prompts or RAG can be A/B tested.

**Depends on:** Phase 1 (baseline prompt) + Phase 2 (RAG corpus)

#### Components to Build

1. **Eval test suite** — A set of standard prompts with expected outcomes:
   ```
   test/eval/eks-basic.txt          → "Generate a production EKS cluster"
   test/eval/s3-versioning.txt      → "Generate an S3 bucket with versioning"
   test/eval/aks-workload-id.txt    → "Generate AKS with workload identity"
   ```

2. **Eval runner** — A Make target or Go test that:
   - Runs each eval prompt through `tfai generate`
   - Saves output to `test/eval/<name>/output/`
   - Runs a checklist validator against each output (file exists? variables have descriptions? encryption present?)

3. **Eval checklist validator** — A simple Go program or script that reads
   generated `.tf` files and checks:
   - [ ] All variables have `description`
   - [ ] All variables have `type`
   - [ ] All outputs have `description`
   - [ ] No dead variables (declared but not referenced)
   - [ ] `tags` variable exists and is wired to resources
   - [ ] Provider-specific checks (e.g., `encryption_config` for EKS)

4. **Eval report** — Output a pass/fail summary per prompt:
   ```
   eks-basic:       12/15 checks passed (missing: OIDC, add-ons, IMDSv2)
   s3-versioning:   10/10 checks passed
   aks-workload-id:  8/12 checks passed (missing: diagnostic settings, tags, CMK, network policy)
   ```

#### Files to Create
- `test/eval/` — eval prompts directory
- `internal/eval/` or `cmd/tfai/commands/eval.go` — eval runner
- `internal/eval/checklist.go` — HCL checklist validator

#### Validation
- Run eval suite against current prompt → establish baseline scores
- Run eval suite after Phase 1 → confirm improvement
- Run eval suite after Phase 2 → confirm further improvement
- `make gate`

---

### Phase 4: Prompt Profiles (Branch: `feat/prompt-profiles`)

**Goal:** Per-model prompt tuning so reasoning models, chat models, and
compact models each get optimized instructions.

**Depends on:** Phase 3 (eval framework must exist to measure profile impact)

#### Components to Build

1. **Profile struct:**
   ```go
   type PromptProfile struct {
       Name       string // "reasoning", "chat", "compact"
       Preamble   string // model-specific instruction style
       Checklist  string // self-audit instructions
   }
   ```

2. **Profile selection** — Auto-detect from `MODEL_PROVIDER` + deployment
   name, overridable via `TFAI_PROMPT_PROFILE` env var.

3. **Profile definitions:**
   - **reasoning** (o-series, codex) — explicit chain-of-thought: "enumerate
     all security, networking, and IAM concerns before generating any HCL"
   - **chat** (gpt-4o, gpt-4.1) — structured output template, concise rules
   - **compact** (gpt-4o-mini, ollama) — shorter prompt, more examples, fewer
     abstract instructions

#### Files to Modify
- `internal/agent/prompt.go` (new) — profile definitions and selection
- `internal/agent/agent.go` — use profile in `buildMessages`
- `internal/provider/factory.go` — expose model info for profile selection

#### Validation
- Run eval suite with each profile on each model type
- Compare scores across profiles
- `make gate`

---

## Execution Order

```
Phase 1 (System Prompt v2)
  ↓ commit + PR + merge + tag
Phase 2 (RAG Pipeline)
  ↓ commit + PR + merge + tag
Phase 3 (Eval Framework)
  ↓ commit + PR + merge + tag
Phase 4 (Prompt Profiles)
  ↓ commit + PR + merge + tag
```

Each phase is independently valuable:
- Phase 1 alone will noticeably improve output quality
- Phase 2 alone will ground the model in real resource schemas
- Phase 3 alone will give you measurable quality metrics
- Phase 4 alone will optimize for your specific model

### Phase 5: Dynamic Context Fetching (Deferred — GH Issue)

**Goal:** Eliminate manual ingestion and framework lookup by having the agent
detect what it needs and fetch it automatically.

#### Two Capabilities

1. **Auto-ingest on demand** — detect resource keywords in the prompt
   (e.g. "EKS cluster", "S3 bucket") → resolve to Terraform Registry URLs →
   ingest into Qdrant → generate. Eliminates `make ingest-aws` as a
   prerequisite.

2. **Framework detection** — detect framework keywords (e.g. "Atmos
   conventions", "Terragrunt") → make a secondary LLM call or web fetch to
   understand the framework structure → inject as RAG context → generate.

#### Security Gate
Both capabilities must be **opt-in** via env var (`TFAI_AUTO_FETCH=true`).
Auto-fetching from external URLs is a supply-chain risk and must never be
enabled by default.

#### Prerequisites
- Phase 2 (manual RAG) must be validated first — auto-ingest is an extension
  of a working RAG pipeline, not a replacement for it.
- Phase 3 (eval framework) must exist to measure whether auto-fetch actually
  improves output quality vs manual ingestion.

#### Sequencing
```
Phase 2: Manual RAG (validate it works)
  ↓
Phase 2.5: Auto-ingest on demand (extend RAG trigger)
  ↓
Phase 5: Framework detection + dynamic context fetch
```

---

## What We're NOT Building Yet

These are intentionally deferred to keep scope tight:

- **MCP server integration** — advanced tool calling, out of scope for prompt/RAG work
- **Multi-model routing** — selecting different models for different tasks
- **Fine-tuning** — prompt engineering + RAG should be exhausted first
- **Plugin system** — custom provider plugins, custom checklist plugins
- **CI/CD eval pipeline** — eval runs in CI on every PR (Phase 3 is local-first)

## Success Criteria

After all four phases, `tfai generate "production EKS cluster"` should produce
a module that:
- Passes 90%+ of the eval checklist
- Includes encryption, IRSA, managed add-ons, tagging, logging
- Has every variable described with sane defaults
- Has every resource commented with purpose
- Would survive a code review from a Senior Platform Engineer
- Scores measurably higher than the current output on the eval framework
