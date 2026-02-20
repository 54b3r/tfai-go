// Package store provides a SQLite-backed conversation history store for the
// TF-AI agent. Each workspace directory has its own conversation thread.
// Messages are persisted across server restarts and injected into the LLM
// context window on subsequent queries.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// Role identifies the author of a conversation message.
type Role string

const (
	// RoleUser is a message sent by the human operator.
	RoleUser Role = "user"
	// RoleAssistant is a message produced by the LLM agent.
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	// Role is the author of the message.
	Role Role
	// Content is the text of the message.
	Content string
	// CreatedAt is when the message was persisted.
	CreatedAt time.Time
}

// ConversationStore persists and retrieves conversation history keyed by
// workspace directory. Implementations must be safe for concurrent use.
type ConversationStore interface {
	// Append persists a single message for the given workspace.
	Append(ctx context.Context, workspaceDir string, role Role, content string) error
	// Recent returns the most recent n messages for the workspace, ordered
	// oldest-first so they can be prepended to the LLM message slice directly.
	// If fewer than n messages exist, all are returned.
	Recent(ctx context.Context, workspaceDir string, n int) ([]Message, error)
	// Close releases any resources held by the store.
	Close() error
}

// SQLiteStore is a ConversationStore backed by a local SQLite database.
type SQLiteStore struct {
	// db is the underlying database connection pool.
	db *sql.DB
}

// DefaultDBPath returns the default path for the conversation history database.
// It resolves to ~/.tfai/history.db, creating the directory if needed.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: could not determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".tfai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("store: could not create %s: %w", dir, err)
	}
	return filepath.Join(dir, "history.db"), nil
}

// Open opens (or creates) a SQLiteStore at the given path and runs the schema
// migration. Use ":memory:" for an in-memory database in tests.
func Open(path string) (*SQLiteStore, error) {
	// WAL mode improves concurrent read performance and is safe for single-host use.
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// Limit to a single writer connection to avoid SQLITE_BUSY under concurrent writes.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate creates the schema if it does not already exist.
func (s *SQLiteStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS conversations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace    TEXT    NOT NULL,
    role         TEXT    NOT NULL CHECK(role IN ('user','assistant')),
    content      TEXT    NOT NULL,
    created_at   INTEGER NOT NULL  -- Unix timestamp (seconds)
);
CREATE INDEX IF NOT EXISTS idx_conversations_workspace_created
    ON conversations (workspace, created_at);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Append persists a single message for the given workspace.
func (s *SQLiteStore) Append(ctx context.Context, workspaceDir string, role Role, content string) error {
	const q = `INSERT INTO conversations (workspace, role, content, created_at) VALUES (?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, workspaceDir, string(role), content, time.Now().Unix()); err != nil {
		return fmt.Errorf("store: append: %w", err)
	}
	return nil
}

// Recent returns the most recent n messages for the workspace, ordered
// oldest-first. Uses a subquery to select the tail then re-order for injection.
func (s *SQLiteStore) Recent(ctx context.Context, workspaceDir string, n int) ([]Message, error) {
	const q = `
SELECT role, content, created_at FROM (
    SELECT id, role, content, created_at
    FROM   conversations
    WHERE  workspace = ?
    ORDER  BY created_at DESC, id DESC
    LIMIT  ?
) ORDER BY created_at ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, q, workspaceDir, n)
	if err != nil {
		return nil, fmt.Errorf("store: recent: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var ts int64
		var role string
		if err := rows.Scan(&role, &m.Content, &ts); err != nil {
			return nil, fmt.Errorf("store: recent scan: %w", err)
		}
		m.Role = Role(role)
		m.CreatedAt = time.Unix(ts, 0)
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: recent rows: %w", err)
	}
	return msgs, nil
}

// Close releases the database connection pool.
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}
