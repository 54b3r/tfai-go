package embedder

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// knownChatModelPrefixes contains name fragments that identify chat/completion
// models which are NOT suitable for embedding. If EMBEDDING_MODEL matches any
// of these, a warning is emitted so the operator knows they may have
// misconfigured the pipeline.
var knownChatModelPrefixes = []string{
	"gpt-4",
	"gpt-3.5",
	"gpt-35",
	"o1",
	"o3",
	"llama3",
	"llama2",
	"llama-3",
	"llama-2",
	"mistral",
	"mixtral",
	"gemma",
	"phi-",
	"phi3",
	"claude",
	"command-r",
	"deepseek",
	"qwen",
	"solar",
	"vicuna",
	"falcon",
	"yi-",
}

// looksLikeChatModel returns true when the model name resembles a known
// chat/completion model rather than a dedicated embedding model.
func looksLikeChatModel(model string) bool {
	lower := strings.ToLower(model)
	for _, prefix := range knownChatModelPrefixes {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

// ValidateForRAG checks that the embedder configuration is safe to use when
// QDRANT_HOST is set. It returns an error if the configuration is clearly
// broken (e.g. azure embedder with no API key), and logs a warning if
// EMBEDDING_MODEL looks like a chat model rather than an embedding model.
//
// This is a pre-flight check — call it before constructing the embedder or
// the Qdrant store so operators get a clear error at startup rather than a
// cryptic failure during the first embed call.
func ValidateForRAG(log *slog.Logger) error {
	qdrantHost := os.Getenv("QDRANT_HOST")
	if qdrantHost == "" {
		// RAG not configured — nothing to validate.
		return nil
	}

	// Resolve the effective embedding backend.
	backend := os.Getenv("EMBEDDING_PROVIDER")
	if backend == "" {
		backend = getEnvOrDefault("MODEL_PROVIDER", "ollama")
	}

	// Warn if the resolved backend is a chat provider with no explicit
	// EMBEDDING_PROVIDER override — the user may have forgotten to set it.
	if backend != "ollama" && os.Getenv("EMBEDDING_PROVIDER") == "" {
		log.Warn("embedder: QDRANT_HOST is set but EMBEDDING_PROVIDER is not — "+
			"inheriting MODEL_PROVIDER as embedding backend",
			slog.String("backend", backend),
			slog.String("hint", "set EMBEDDING_PROVIDER=ollama (or openai/azure) to be explicit"),
		)
	}

	// Validate backend-specific required config.
	switch backend {
	case "openai":
		apiKey := os.Getenv("EMBEDDING_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			return fmt.Errorf("embedder: QDRANT_HOST is set but no OpenAI API key found — set OPENAI_API_KEY or EMBEDDING_API_KEY")
		}

	case "azure":
		apiKey := os.Getenv("EMBEDDING_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
		}
		if apiKey == "" {
			return fmt.Errorf("embedder: QDRANT_HOST is set but no Azure API key found — set AZURE_OPENAI_API_KEY or EMBEDDING_API_KEY")
		}
		endpoint := os.Getenv("EMBEDDING_ENDPOINT")
		if endpoint == "" {
			endpoint = os.Getenv("AZURE_OPENAI_ENDPOINT")
		}
		if endpoint == "" {
			return fmt.Errorf("embedder: QDRANT_HOST is set but no Azure endpoint found — set AZURE_OPENAI_ENDPOINT or EMBEDDING_ENDPOINT")
		}

	case "bedrock":
		return fmt.Errorf("embedder: QDRANT_HOST is set but bedrock embedding is not yet implemented — set EMBEDDING_PROVIDER to ollama, openai, or azure")

	case "gemini":
		return fmt.Errorf("embedder: QDRANT_HOST is set but gemini embedding is not yet implemented — set EMBEDDING_PROVIDER to ollama, openai, or azure")
	}

	// Warn if EMBEDDING_MODEL looks like a chat model.
	model := os.Getenv("EMBEDDING_MODEL")
	if model != "" && looksLikeChatModel(model) {
		log.Warn("embedder: EMBEDDING_MODEL looks like a chat model, not an embedding model — "+
			"this will likely produce poor or broken embeddings",
			slog.String("model", model),
			slog.String("hint", "use a dedicated embedding model e.g. nomic-embed-text, text-embedding-3-small"),
		)
	}

	return nil
}
