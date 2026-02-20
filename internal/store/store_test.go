package store

import (
	"context"
	"testing"
)

// openTestStore opens an in-memory SQLiteStore for use in tests.
func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func Test_Store_AppendAndRecent(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.Append(ctx, "/ws/a", RoleUser, "hello"); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := s.Append(ctx, "/ws/a", RoleAssistant, "world"); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	msgs, err := s.Recent(ctx, "/ws/a", 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != RoleUser || msgs[0].Content != "hello" {
		t.Errorf("msg[0]: want user/hello, got %s/%s", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != RoleAssistant || msgs[1].Content != "world" {
		t.Errorf("msg[1]: want assistant/world, got %s/%s", msgs[1].Role, msgs[1].Content)
	}
}

func Test_Store_RecentLimitRespected(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ctx := context.Background()

	for i := range 6 {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAssistant
		}
		if err := s.Append(ctx, "/ws/b", role, "msg"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	msgs, err := s.Recent(ctx, "/ws/b", 4)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(msgs) != 4 {
		t.Errorf("want 4 messages, got %d", len(msgs))
	}
}

func Test_Store_WorkspaceIsolation(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.Append(ctx, "/ws/x", RoleUser, "from x"); err != nil {
		t.Fatalf("append x: %v", err)
	}
	if err := s.Append(ctx, "/ws/y", RoleUser, "from y"); err != nil {
		t.Fatalf("append y: %v", err)
	}

	msgsX, err := s.Recent(ctx, "/ws/x", 10)
	if err != nil {
		t.Fatalf("recent x: %v", err)
	}
	msgsY, err := s.Recent(ctx, "/ws/y", 10)
	if err != nil {
		t.Fatalf("recent y: %v", err)
	}

	if len(msgsX) != 1 || msgsX[0].Content != "from x" {
		t.Errorf("workspace x isolation failed: got %v", msgsX)
	}
	if len(msgsY) != 1 || msgsY[0].Content != "from y" {
		t.Errorf("workspace y isolation failed: got %v", msgsY)
	}
}

func Test_Store_EmptyWorkspaceReturnsNil(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ctx := context.Background()

	msgs, err := s.Recent(ctx, "/ws/empty", 10)
	if err != nil {
		t.Fatalf("recent empty: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("want 0 messages, got %d", len(msgs))
	}
}

func Test_Store_OldestFirstOrdering(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)
	ctx := context.Background()

	contents := []string{"first", "second", "third"}
	for _, c := range contents {
		if err := s.Append(ctx, "/ws/order", RoleUser, c); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	msgs, err := s.Recent(ctx, "/ws/order", 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	for i, want := range contents {
		if msgs[i].Content != want {
			t.Errorf("msg[%d]: want %q, got %q", i, want, msgs[i].Content)
		}
	}
}
