// Package audit provides a structured audit logger for CLI command invocations.
// It logs command name, resolved configuration, and sanitised environment state
// so operators can trace what happened without exposing secret values.
//
// Secrets are logged as presence/absence only — never their values.
package audit

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// secretEnvKeys lists environment variable names whose values must never be
// logged. Only presence ("set") or absence ("unset") is recorded.
var secretEnvKeys = map[string]bool{
	"OPENAI_API_KEY":        true,
	"AZURE_OPENAI_API_KEY":  true,
	"GOOGLE_API_KEY":        true,
	"EMBEDDING_API_KEY":     true,
	"QDRANT_API_KEY":        true,
	"TFAI_API_KEY":          true,
	"LANGFUSE_PUBLIC_KEY":   true,
	"LANGFUSE_SECRET_KEY":   true,
	"AWS_SECRET_ACCESS_KEY": true,
	"AWS_SESSION_TOKEN":     true,
}

// LogCommandStart emits a structured audit log entry when a CLI command begins.
// It records the command name, config file source, and sanitised environment.
func LogCommandStart(log *slog.Logger, command string, configPath string) {
	attrs := []slog.Attr{
		slog.String("command", command),
		slog.String("config_file", sanitiseConfigPath(configPath)),
	}

	// Log key operational env vars with sanitisation.
	for _, entry := range auditKeys {
		val := os.Getenv(entry.key)
		if entry.secret {
			attrs = append(attrs, slog.String(entry.key, presence(val)))
		} else {
			attrs = append(attrs, slog.String(entry.key, valOrUnset(val)))
		}
	}

	log.LogAttrs(context.TODO(), slog.LevelInfo, "audit: command start", attrs...)
}

// auditEntry defines an env var to include in the audit log.
type auditEntry struct {
	// key is the environment variable name.
	key string
	// secret indicates the value should be redacted to presence/absence.
	secret bool
}

// auditKeys is the ordered list of env vars included in every audit log entry.
var auditKeys = []auditEntry{
	{"MODEL_PROVIDER", false},
	{"OLLAMA_HOST", false},
	{"OLLAMA_MODEL", false},
	{"OPENAI_API_KEY", true},
	{"OPENAI_MODEL", false},
	{"AZURE_OPENAI_API_KEY", true},
	{"AZURE_OPENAI_ENDPOINT", false},
	{"AZURE_OPENAI_DEPLOYMENT", false},
	{"GOOGLE_API_KEY", true},
	{"GEMINI_MODEL", false},
	{"AWS_REGION", false},
	{"BEDROCK_MODEL_ID", false},
	{"EMBEDDING_PROVIDER", false},
	{"EMBEDDING_MODEL", false},
	{"EMBEDDING_API_KEY", true},
	{"QDRANT_HOST", false},
	{"QDRANT_PORT", false},
	{"QDRANT_COLLECTION", false},
	{"QDRANT_API_KEY", true},
	{"TFAI_API_KEY", true},
	{"TFAI_HISTORY_DB", false},
	{"LOG_LEVEL", false},
	{"LOG_FORMAT", false},
	{"LANGFUSE_PUBLIC_KEY", true},
	{"LANGFUSE_SECRET_KEY", true},
}

// SanitiseKey returns "set" or "unset" for known secret keys, or the actual
// value for non-secret keys. This is safe to use in log messages.
func SanitiseKey(key, value string) string {
	if secretEnvKeys[key] {
		return presence(value)
	}
	return valOrUnset(value)
}

// presence returns "set" if the value is non-empty, "unset" otherwise.
func presence(v string) string {
	if v != "" {
		return "set"
	}
	return "unset"
}

// valOrUnset returns the value if non-empty, "unset" otherwise.
func valOrUnset(v string) string {
	if v != "" {
		return v
	}
	return "unset"
}

// sanitiseConfigPath returns the config path or "none" if empty.
func sanitiseConfigPath(p string) string {
	if p == "" {
		return "none"
	}
	// Redact home directory for privacy in logs.
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
