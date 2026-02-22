package audit

import (
	"os"
	"testing"
)

func TestSanitiseKey_Secret(t *testing.T) {
	t.Parallel()
	if got := SanitiseKey("OPENAI_API_KEY", "sk-abc123"); got != "set" {
		t.Errorf("expected 'set', got %q", got)
	}
	if got := SanitiseKey("OPENAI_API_KEY", ""); got != "unset" {
		t.Errorf("expected 'unset', got %q", got)
	}
}

func TestSanitiseKey_NonSecret(t *testing.T) {
	t.Parallel()
	if got := SanitiseKey("MODEL_PROVIDER", "azure"); got != "azure" {
		t.Errorf("expected 'azure', got %q", got)
	}
	if got := SanitiseKey("MODEL_PROVIDER", ""); got != "unset" {
		t.Errorf("expected 'unset', got %q", got)
	}
}

func TestPresence(t *testing.T) {
	t.Parallel()
	if got := presence("something"); got != "set" {
		t.Errorf("expected 'set', got %q", got)
	}
	if got := presence(""); got != "unset" {
		t.Errorf("expected 'unset', got %q", got)
	}
}

func TestSanitiseConfigPath(t *testing.T) {
	t.Parallel()
	if got := sanitiseConfigPath(""); got != "none" {
		t.Errorf("expected 'none', got %q", got)
	}
	if got := sanitiseConfigPath("/tmp/config.yaml"); got != "/tmp/config.yaml" {
		t.Errorf("expected '/tmp/config.yaml', got %q", got)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := home + "/.tfai/config.yaml"
		if got := sanitiseConfigPath(p); got != "~/.tfai/config.yaml" {
			t.Errorf("expected '~/.tfai/config.yaml', got %q", got)
		}
	}
}
