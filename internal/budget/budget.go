// Package budget provides token budget estimation and message trimming for the
// TF-AI agent. Because the agent supports multiple LLM backends with different
// tokenizers, this package uses a conservative character-based heuristic:
// 1 token ≈ 4 characters (English prose and code). This deliberately
// under-estimates token counts to leave headroom for model-specific overhead.
package budget

import (
	"github.com/cloudwego/eino/schema"
)

const (
	// charsPerToken is the conservative character-to-token ratio used for
	// estimation. 4 chars/token is standard for English and code; using 3
	// would be more aggressive but risks overflowing context windows.
	charsPerToken = 4

	// DefaultMaxContextTokens is the default input context budget in tokens.
	// Conservative enough to fit within 8k-context models (Llama 3 8B, GPT-3.5)
	// while leaving room for the output. Override via Config.MaxContextTokens.
	DefaultMaxContextTokens = 6000
)

// Estimate returns a rough token count for s using the character heuristic.
func Estimate(s string) int {
	n := len(s) / charsPerToken
	if n == 0 && len(s) > 0 {
		return 1
	}
	return n
}

// EstimateMessages returns the estimated total token count for a slice of
// schema.Message values, summing role + content for each message.
func EstimateMessages(msgs []*schema.Message) int {
	total := 0
	for _, m := range msgs {
		// Each message has a small per-message overhead (~4 tokens in most APIs).
		total += 4
		total += Estimate(string(m.Role))
		total += Estimate(m.Content)
	}
	return total
}

// TrimHistory removes the oldest messages from history until the total
// estimated token count of fixed + history + current fits within maxTokens.
// fixed contains messages that must not be trimmed (system prompt, RAG context,
// workspace context, current user message). history contains prior conversation
// turns that may be dropped oldest-first.
//
// Returns the trimmed history slice. If even an empty history exceeds the
// budget, the empty slice is returned (fixed messages are never dropped here —
// callers should warn separately if fixed alone exceeds the budget).
func TrimHistory(fixed, history []*schema.Message, maxTokens int) []*schema.Message {
	if len(history) == 0 {
		return history
	}

	fixedTokens := EstimateMessages(fixed)

	// Binary search would be more efficient but history is typically ≤20 msgs;
	// linear scan from the front (dropping oldest) is clear and correct.
	for len(history) > 0 {
		if fixedTokens+EstimateMessages(history) <= maxTokens {
			break
		}
		// Drop the oldest message.
		history = history[1:]
	}
	return history
}
