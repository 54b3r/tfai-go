package budget

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func Test_Estimate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},        // < 4 chars → 1
		{"abcd", 1},     // exactly 4 chars → 1
		{"abcde", 1},    // 5 chars → 1
		{"abcdefgh", 2}, // 8 chars → 2
		{strings.Repeat("x", 400), 100},
	}
	for _, tc := range cases {
		got := Estimate(tc.input)
		if got != tc.want {
			t.Errorf("Estimate(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func Test_EstimateMessages(t *testing.T) {
	t.Parallel()
	msgs := []*schema.Message{
		schema.UserMessage("hello world"), // 4 overhead + 1 (role) + 2 (content) = 7
		schema.UserMessage("hello world"),
	}
	got := EstimateMessages(msgs)
	// Each message: 4 overhead + Estimate("user")=1 + Estimate("hello world")=2 = 7
	// Two messages: 14
	if got != 14 {
		t.Errorf("EstimateMessages = %d, want 14", got)
	}
}

func Test_TrimHistory_NoTrimNeeded(t *testing.T) {
	t.Parallel()
	fixed := []*schema.Message{schema.SystemMessage("sys")}
	history := []*schema.Message{
		schema.UserMessage("hi"),
		schema.UserMessage("there"),
	}
	got := TrimHistory(fixed, history, DefaultMaxContextTokens)
	if len(got) != 2 {
		t.Errorf("want 2 history messages, got %d", len(got))
	}
}

func Test_TrimHistory_DropsOldest(t *testing.T) {
	t.Parallel()
	history := []*schema.Message{
		schema.UserMessage("oldest"),
		schema.UserMessage("newest"),
	}
	// Each history message costs: 4 overhead + Estimate("user")=1 + Estimate(content)=1 = 6 tokens.
	// Two messages = 12 tokens. One message = 6 tokens.
	// Set fixed to an empty slice and budget to 7 — fits exactly one message (6 ≤ 7)
	// but not two (12 > 7). The oldest should be dropped.
	fixed := []*schema.Message{}
	got := TrimHistory(fixed, history, 7)
	if len(got) != 1 {
		t.Errorf("want 1 history message after trim, got %d", len(got))
	}
	if got[0].Content != "newest" {
		t.Errorf("want newest message retained, got %q", got[0].Content)
	}
}

func Test_TrimHistory_EmptyHistory(t *testing.T) {
	t.Parallel()
	fixed := []*schema.Message{schema.SystemMessage("sys")}
	got := TrimHistory(fixed, nil, DefaultMaxContextTokens)
	if len(got) != 0 {
		t.Errorf("want empty, got %d", len(got))
	}
}

func Test_TrimHistory_AllDroppedWhenFixedExceedsBudget(t *testing.T) {
	t.Parallel()
	// Fixed alone exceeds budget — all history should be dropped.
	fixed := []*schema.Message{
		schema.SystemMessage(strings.Repeat("x", 4*7000)), // ~7000 tokens
	}
	history := []*schema.Message{
		schema.UserMessage("a"),
		schema.UserMessage("b"),
	}
	got := TrimHistory(fixed, history, 6000)
	if len(got) != 0 {
		t.Errorf("want 0 history messages, got %d", len(got))
	}
}
