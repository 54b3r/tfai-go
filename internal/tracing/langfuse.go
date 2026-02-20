package tracing

import (
	"context"
	"os"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/callbacks"

	"github.com/54b3r/tfai-go/internal/version"
)

// Setup initialises the Langfuse callback handler if LANGFUSE_PUBLIC_KEY and
// LANGFUSE_SECRET_KEY are set. Returns a flush function that must be called
// before process exit to ensure all traces are sent. If Langfuse is not
// configured, both return values are nil and tracing is silently disabled.
func Setup() (callbacks.Handler, func(), bool) {
	host := os.Getenv("LANGFUSE_HOST")
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	if publicKey == "" || secretKey == "" {
		return nil, nil, false
	}
	if host == "" {
		host = "http://localhost:3000"
	}

	handler, flusher := langfuse.NewLangfuseHandler(&langfuse.Config{
		Host:      host,
		PublicKey: publicKey,
		SecretKey: secretKey,
		Name:      "tfai",
		Release:   version.Version,
		Tags:      []string{"tfai", "terraform", "llm"},
	})

	return handler, flusher, true
}

// SetRequestTrace stamps the context with per-request trace metadata so each
// chat request appears as a distinct, named trace in Langfuse. Call this once
// per request before invoking the agent. sessionID should be a unique ID for
// the request (e.g. a UUID or the HTTP request ID).
func SetRequestTrace(ctx context.Context, sessionID string) context.Context {
	return langfuse.SetTrace(ctx,
		langfuse.WithName("tfai-chat"),
		langfuse.WithSessionID(sessionID),
		langfuse.WithRelease(version.Version),
		langfuse.WithTags("tfai", "chat"),
	)
}
