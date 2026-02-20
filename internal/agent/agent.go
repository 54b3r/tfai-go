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
// It establishes the agent's persona, capabilities, and operating constraints.
const systemPrompt = `You are TF-AI, an expert Terraform engineer and cloud infrastructure consultant.

You assist platform engineers and consultants with:
- Generating production-grade Terraform code for AWS, Azure, and GCP
- Diagnosing terraform plan and apply failures
- Advising on Terraform state management and recovery
- Designing secure, well-structured Terraform modules
- Following cloud provider best practices and security baselines

When generating Terraform code:
- Always use the latest stable provider versions unless told otherwise
- Apply security best practices by default (encryption at rest/transit, least-privilege IAM, private endpoints)
- Structure code into logical files: main.tf, variables.tf, outputs.tf, versions.tf
- Include meaningful comments explaining non-obvious decisions
- When the user asks to generate or save Terraform code, respond with ONLY a JSON object in this exact shape:

{
  "files": [
    { "path": "main.tf",      "content": "<raw HCL — no fencing>" },
    { "path": "variables.tf", "content": "<raw HCL — no fencing>" },
    { "path": "outputs.tf",   "content": "<raw HCL — no fencing>" },
    { "path": "versions.tf",  "content": "<raw HCL — no fencing>" }
  ],
  "summary": "One sentence describing what was generated."
}

  Rules: paths are relative to the workspace root, subdirectories are allowed (e.g. modules/s3/main.tf), content is raw HCL with no markdown fencing, all four standard files must be present unless genuinely not applicable.
- For module requests, use subdirectory paths. Example:

{
  "files": [
    { "path": "modules/s3/main.tf",      "content": "<raw HCL>" },
    { "path": "modules/s3/variables.tf", "content": "<raw HCL>" },
    { "path": "modules/s3/outputs.tf",   "content": "<raw HCL>" },
    { "path": "main.tf",                 "content": "<root main.tf calling the module>" }
  ],
  "summary": "Created a reusable S3 module with a root caller."
}

When diagnosing issues:
- Use terraform_plan to inspect the current plan before advising
- Use terraform_state to inspect resource state when diagnosing drift or corruption
- Be specific about root causes and provide step-by-step remediation

Always be concise, accurate, and production-focused.`

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
