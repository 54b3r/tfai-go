// Package agent wires together the Eino ChatModelAgent with the Terraform-specific
// tools and RAG retriever to form the core TF-AI assistant.
// The agent handles the full ReAct loop: it decides when to call tools,
// when to query the RAG retriever for context, and when to respond directly.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/54b3r/tfai-go/internal/budget"
	"github.com/54b3r/tfai-go/internal/logging"
	"github.com/54b3r/tfai-go/internal/rag"
	"github.com/54b3r/tfai-go/internal/store"
)

// systemPrompt is the base system prompt injected into every conversation.
// It establishes the agent's persona, engineering philosophy, production
// baseline, module design standards, and self-audit requirements.
const systemPrompt = `You are TF-AI, a Principal Staff Engineer operating at the intersection of
Platform Engineering, SRE, and Cloud Architecture. You have deep, first-principles
expertise in Terraform, cloud infrastructure, and infrastructure-as-code at
enterprise scale across AWS, Azure, and GCP.

You do not just answer questions — you think several steps ahead of the operator.
Your job is to produce infrastructure that is secure by default, observable from
day one, reusable across teams, and maintainable by the engineer who inherits it
two years from now. You treat every generate request as if it will be reviewed by
a Principal Engineer, audited by a security team, and operated by an on-call SRE
at 3am.

You hold yourself to these non-negotiable standards:
- Security is not a feature to add later — it is the baseline
- Every resource you create must be explainable to a compliance auditor
- Operational concerns (logging, tagging, lifecycle, recovery) are first-class, not afterthoughts
- Reusability and clean module interfaces matter as much as correctness
- You flag tradeoffs explicitly — you never silently choose convenience over security

## Your Capabilities

- Generate production-grade Terraform modules for AWS, Azure, and GCP
- Diagnose terraform plan and apply failures with root-cause analysis
- Advise on Terraform state management, drift, and recovery
- Design reusable, well-structured modules with clean interfaces
- Apply cloud provider security baselines and compliance frameworks (CIS, NIST)

## How You Think Before Generating

Before writing a single line of HCL, mentally enumerate ALL of the following
concerns for the resource type being requested. Every applicable item must be
addressed in the output — not skipped, not left as a TODO:

1. **Encryption** — data at rest (KMS/CMEK), data in transit (TLS, private endpoints)
2. **IAM** — least-privilege roles, explicit trust policies, no wildcard permissions
3. **Networking** — private subnets, explicit security groups with minimal ingress/egress, no 0.0.0.0/0 unless explicitly required
4. **Observability** — logging enabled, audit trails, metrics hooks where applicable
5. **Tagging** — every resource tagged for cost allocation, environment, ownership, and compliance
6. **Lifecycle** — deletion protection on stateful resources, backup policies, retention periods
7. **Provider-specific hardening** — e.g. IMDSv2 for EC2/EKS nodes, IRSA for EKS workloads, managed add-ons for EKS, diagnostic settings for Azure, CMEK for GCP

## Module Design Philosophy

Every module you generate must be reusable and operator-friendly:

- **Variables**: every variable has a ` + "`description`" + `, a ` + "`type`" + `, and a ` + "`default`" + ` where a sane default exists.
  Use ` + "`validation`" + ` blocks for variables with constrained value sets.
  No dead variables — every declared variable must be referenced in a resource.
- **Outputs**: every output has a ` + "`description`" + `. Expose what downstream callers need:
  IDs, ARNs, endpoints, OIDC issuer URLs, role ARNs — anything a dependent module or
  CI/CD pipeline would need to reference.
- **Comments**: every resource and module block has a comment above it explaining
  its PURPOSE and any non-obvious decisions (not just its type). Use section headers
  to group related resources:
  ` + "`# ── IAM ──────────────────────────────────────────────────────────────────`" + `
- **Formatting**: align ` + "`=`" + ` signs within each block, one attribute per line,
  blank lines between blocks. HCL must be ` + "`terraform fmt`" + `-clean.
- **File structure**: split into logical files. Standard layout:
  - ` + "`main.tf`" + ` — resources
  - ` + "`variables.tf`" + ` — input variables
  - ` + "`outputs.tf`" + ` — output values
  - ` + "`versions.tf`" + ` — terraform and provider version constraints
  - ` + "`locals.tf`" + ` — local values (if needed)
  - ` + "`data.tf`" + ` — data sources (if needed)

## Self-Audit Before Responding

Before returning generated code, verify:
- [ ] Every item in the "How You Think" checklist above is addressed or explicitly noted as not applicable with a reason
- [ ] No dead variables (every variable is used in at least one resource)
- [ ] Every output a downstream consumer would need is present
- [ ] Tags variable exists and is applied to every taggable resource
- [ ] Security groups have explicit rules — no implicit defaults relied upon
- [ ] Stateful resources have deletion protection or lifecycle policies
- [ ] The code would pass a code review from a Senior Platform Engineer

## Output Format for Code Generation

When the user asks to generate or save Terraform code, respond with ONLY a
JSON object in this exact shape — no markdown fencing, no explanation outside
the JSON:

{
  "files": [
    { "path": "main.tf",      "content": "<raw HCL — no markdown fencing>" },
    { "path": "variables.tf", "content": "<raw HCL — no markdown fencing>" },
    { "path": "outputs.tf",   "content": "<raw HCL — no markdown fencing>" },
    { "path": "versions.tf",  "content": "<raw HCL — no markdown fencing>" }
  ],
  "summary": "One sentence describing what was generated and the key security decisions made."
}

Rules:
- Paths are relative to the workspace root
- Subdirectories are allowed and encouraged for modules: ` + "`modules/eks/main.tf`" + `
- Content is raw HCL with no markdown fencing
- All four standard files must be present unless genuinely not applicable
- The summary must mention the key security decisions (e.g. "KMS encryption, private endpoints, IRSA enabled")

For module requests with a root caller:

{
  "files": [
    { "path": "modules/eks/main.tf",      "content": "<raw HCL>" },
    { "path": "modules/eks/variables.tf", "content": "<raw HCL>" },
    { "path": "modules/eks/outputs.tf",   "content": "<raw HCL>" },
    { "path": "modules/eks/versions.tf",  "content": "<raw HCL>" },
    { "path": "main.tf",                  "content": "<root main.tf calling the module>" },
    { "path": "variables.tf",             "content": "<root variables>" }
  ],
  "summary": "Created a reusable EKS module with KMS encryption, IRSA, managed add-ons, and a root caller."
}

## Diagnosing Issues

- Use terraform_plan to inspect the current plan before advising
- Use terraform_state to inspect resource state when diagnosing drift or corruption
- Always identify the root cause — not just the symptom
- Provide step-by-step remediation with the exact commands to run
- Note any state surgery risks before recommending ` + "`terraform state`" + ` commands

## General Standards

- Use the latest stable provider versions unless the user specifies otherwise
- Never suggest ` + "`ignore_changes`" + ` without explaining the operational risk
- Never use ` + "`count`" + ` for resources that have identity — use ` + "`for_each`" + ` instead
- Prefer data sources over hardcoded ARNs/IDs
- Flag any decision that trades security for convenience — let the operator decide`

// Config holds the dependencies required to construct a TerraformAgent.
type Config struct {
	// ChatModel is the LLM backend constructed by the provider factory.
	ChatModel model.ToolCallingChatModel

	// Tools is the list of Terraform tools available to the agent.
	Tools []tool.BaseTool

	// Retriever is the RAG retriever for Terraform documentation context.
	// May be nil if RAG is not configured.
	Retriever rag.Retriever

	// RAGTopK controls how many RAG documents are injected per query.
	// Defaults to 5 if zero.
	RAGTopK int
	// History is the optional conversation store used to persist and replay
	// prior turns. If nil, each query is stateless.
	History store.ConversationStore
	// HistoryDepth is the number of prior turns (user+assistant pairs) to
	// inject per query. Defaults to 10 if zero.
	HistoryDepth int
	// MaxContextTokens is the estimated token budget for the full input context
	// (system prompt + history + RAG + workspace + user message). History is
	// trimmed oldest-first to fit. Defaults to budget.DefaultMaxContextTokens
	// if zero.
	MaxContextTokens int
}

// TerraformAgent wraps the Eino ReAct agent with Terraform-specific behaviour,
// including optional RAG context injection before each query.
type TerraformAgent struct {
	// reactAgent is the underlying Eino ReAct loop agent.
	reactAgent *react.Agent

	// retriever is the optional RAG retriever for documentation context.
	retriever rag.Retriever

	// ragTopK is the number of RAG documents to inject per query.
	ragTopK int

	// history is the optional conversation store for multi-turn context.
	history store.ConversationStore

	// historyDepth is the number of recent messages to inject per query.
	historyDepth int

	// maxContextTokens is the estimated token budget for the full input context.
	maxContextTokens int
}

// New constructs a TerraformAgent from the provided Config.
func New(ctx context.Context, cfg *Config) (*TerraformAgent, error) {
	if cfg.ChatModel == nil {
		return nil, fmt.Errorf("agent: ChatModel must not be nil")
	}

	topK := cfg.RAGTopK
	if topK <= 0 {
		topK = 5
	}

	agentCfg := &react.AgentConfig{
		ToolCallingModel: cfg.ChatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: cfg.Tools,
		},
	}

	reactAgent, err := react.NewAgent(ctx, agentCfg)
	if err != nil {
		return nil, fmt.Errorf("agent: failed to create ReAct agent: %w", err)
	}

	depth := cfg.HistoryDepth
	if depth <= 0 {
		depth = 10
	}

	maxCtx := cfg.MaxContextTokens
	if maxCtx <= 0 {
		maxCtx = budget.DefaultMaxContextTokens
	}

	return &TerraformAgent{
		reactAgent:       reactAgent,
		retriever:        cfg.Retriever,
		ragTopK:          topK,
		history:          cfg.History,
		historyDepth:     depth,
		maxContextTokens: maxCtx,
	}, nil
}

// Query sends a user message to the agent and streams the response to the
// provided writer. If a RAG retriever is configured, relevant documentation
// context is prepended to the message before it reaches the LLM.
// If a conversation store is configured, prior turns are injected and the
// new user message and assistant response are persisted after completion.
func (a *TerraformAgent) Query(ctx context.Context, userMessage, workspaceDir string, w io.Writer) (bool, error) {
	filesWritten := false
	messages, err := a.buildMessages(ctx, userMessage, workspaceDir)
	if err != nil {
		return filesWritten, fmt.Errorf("agent: failed to build messages: %w", err)
	}

	sr, err := a.reactAgent.Stream(ctx, messages)
	if err != nil {
		return filesWritten, fmt.Errorf("agent: stream failed: %w", err)
	}
	defer sr.Close()
	var msgBuf strings.Builder
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return filesWritten, fmt.Errorf("agent: stream receive error: %w", err)
		}
		if msg != nil && msg.Content != "" {
			if _, err := fmt.Fprint(&msgBuf, msg.Content); err != nil {
				return filesWritten, fmt.Errorf("agent: write error: %w", err)
			}
		}

	}

	// If a workspace directory was provided, attempt to parse the buffered output
	// as a terraform_generate JSON envelope. On success, write files to disk and
	// stream the human-readable summary to the caller. On failure (regular text
	// response), fall through and stream the raw buffer as normal.
	if workspaceDir != "" {
		result, err := parseAgentOutput(msgBuf.String())
		if err == nil && len(result.Files) > 0 {
			if err := applyFiles(result, workspaceDir); err != nil {
				return filesWritten, fmt.Errorf("agent: Query: failed to apply files: %w", err)
			}
			filesWritten = true
			// Stream the summary to the SSE writer, not stdout.
			fmt.Fprint(w, result.Summary)
			return filesWritten, nil
		}
	}

	// Not a terraform_generate result — stream the raw accumulated content.
	if _, err := fmt.Fprint(w, msgBuf.String()); err != nil {
		return filesWritten, fmt.Errorf("agent: write error: %w", err)
	}

	// Persist the turn to the conversation store (non-fatal on error).
	if a.history != nil {
		if err := a.history.Append(ctx, workspaceDir, store.RoleUser, userMessage); err != nil {
			logging.FromContext(ctx).Warn("history: failed to persist user message", slog.Any("error", err))
		}
		if err := a.history.Append(ctx, workspaceDir, store.RoleAssistant, msgBuf.String()); err != nil {
			logging.FromContext(ctx).Warn("history: failed to persist assistant message", slog.Any("error", err))
		}
	}

	return filesWritten, nil
}

// buildMessages constructs the message slice for the agent, optionally
// prepending RAG context retrieved for the user's query.
func (a *TerraformAgent) buildMessages(ctx context.Context, userMessage, workspaceDir string) ([]*schema.Message, error) {
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
	}

	// Inject recent conversation history so the LLM has multi-turn context.
	// History is trimmed oldest-first to stay within the token budget.
	var historyMsgs []*schema.Message
	if a.history != nil {
		prior, err := a.history.Recent(ctx, workspaceDir, a.historyDepth*2)
		if err != nil {
			logging.FromContext(ctx).Warn("history: failed to load prior messages", slog.Any("error", err))
		} else {
			for _, m := range prior {
				switch m.Role {
				case store.RoleUser:
					historyMsgs = append(historyMsgs, schema.UserMessage(m.Content))
				case store.RoleAssistant:
					historyMsgs = append(historyMsgs, schema.AssistantMessage(m.Content, nil))
				}
			}
		}
	}

	if a.retriever != nil {
		docs, err := a.retriever.Retrieve(ctx, userMessage, a.ragTopK)
		if err != nil {
			// RAG failure is non-fatal — log and continue without context.
			logging.FromContext(ctx).Warn("RAG retrieval failed, continuing without context", slog.Any("error", err))
		} else if len(docs) > 0 {
			ragContext := buildRAGContext(docs)
			messages = append(messages, schema.SystemMessage(ragContext))
		}
	}

	// Inject current workspace file contents so the LLM can read and modify
	// existing files, not just generate new ones from scratch.
	if workspaceDir != "" {
		wsContext, err := buildWorkspaceContext(workspaceDir)
		if err == nil && wsContext != "" {
			messages = append(messages, schema.SystemMessage(wsContext))
		}
	}

	// Add the current user message to the fixed set for budget calculation.
	fixed := append(messages, schema.UserMessage(userMessage)) //nolint:gocritic // intentional copy

	// Trim history oldest-first so the total estimated token count fits within
	// the configured context budget.
	before := len(historyMsgs)
	historyMsgs = budget.TrimHistory(fixed, historyMsgs, a.maxContextTokens)
	if dropped := before - len(historyMsgs); dropped > 0 {
		logging.FromContext(ctx).Warn("budget: dropped history messages to fit context window",
			slog.Int("dropped", dropped),
			slog.Int("retained", len(historyMsgs)),
			slog.Int("max_tokens", a.maxContextTokens),
		)
	}

	// Insert trimmed history between the system prompt and the rest of the fixed
	// messages (RAG context, workspace context, user message).
	// messages currently holds: [system, ...rag, ...workspace]
	// We want: [system, ...history, ...rag, ...workspace, user]
	result := make([]*schema.Message, 0, 1+len(historyMsgs)+len(messages)-1+1)
	result = append(result, messages[0])     // system prompt
	result = append(result, historyMsgs...)  // trimmed history
	result = append(result, messages[1:]...) // RAG + workspace
	result = append(result, schema.UserMessage(userMessage))
	return result, nil
}

// buildWorkspaceContext reads all .tf files in the workspace directory and
// formats them into a system message so the LLM can inspect and modify
// existing Terraform configurations. Returns an empty string if the directory
// contains no .tf files. Non-fatal errors (unreadable files) are skipped.
func buildWorkspaceContext(workspaceDir string) (string, error) {
	var sb strings.Builder

	err := filepath.WalkDir(workspaceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".tf") {
			return nil
		}
		rel, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}
		fmt.Fprintf(&sb, "### %s\n```hcl\n%s\n```\n\n", rel, content)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("agent: workspace walk failed: %w", err)
	}

	if sb.Len() == 0 {
		return "", nil
	}

	return "## Current Workspace Files\n\n" +
		"The following Terraform files are currently in the workspace. " +
		"When the user asks to modify, update, or extend the configuration, " +
		"use these as the base and return the full updated file contents in the JSON envelope.\n\n" +
		sb.String(), nil
}

// buildRAGContext formats retrieved documents into a system message that
// provides the LLM with relevant Terraform documentation context.
func buildRAGContext(docs []rag.Document) string {
	context := "## Relevant Terraform Documentation\n\n" +
		"The following documentation excerpts are relevant to the user's query. " +
		"Use them to inform your response where applicable.\n\n"

	for i, doc := range docs {
		context += fmt.Sprintf("### Source %d: %s\n%s\n\n", i+1, doc.Source, doc.Content)
	}

	return context
}
