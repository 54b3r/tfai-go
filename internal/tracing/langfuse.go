package tracing

import (
	"os"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/callbacks"
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
	})

	return handler, flusher, true
}
