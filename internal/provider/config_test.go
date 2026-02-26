package provider

import (
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		// ── Ollama ────────────────────────────────────────────────────────────
		{
			name: "ollama/valid",
			cfg: Config{
				Backend: BackendOllama,
				Ollama:  ProviderOllama{Host: "http://localhost:11434", Model: "llama3"},
			},
		},
		{
			name:    "ollama/missing model",
			cfg:     Config{Backend: BackendOllama, Ollama: ProviderOllama{Host: "http://localhost:11434"}},
			wantErr: "OLLAMA_MODEL",
		},

		// ── OpenAI ────────────────────────────────────────────────────────────
		{
			name: "openai/valid",
			cfg: Config{
				Backend: BackendOpenAI,
				OpenAI:  ProviderOpenAI{APIKey: "sk-test", Model: "gpt-4o"},
			},
		},
		{
			name:    "openai/missing api key",
			cfg:     Config{Backend: BackendOpenAI, OpenAI: ProviderOpenAI{Model: "gpt-4o"}},
			wantErr: "OPENAI_API_KEY",
		},
		{
			name:    "openai/missing model",
			cfg:     Config{Backend: BackendOpenAI, OpenAI: ProviderOpenAI{APIKey: "sk-test"}},
			wantErr: "OPENAI_MODEL",
		},

		// ── Azure ─────────────────────────────────────────────────────────────
		{
			name: "azure/valid",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:     "key",
					Endpoint:   "https://my.openai.azure.com",
					Deployment: "gpt-4o",
					APIVersion: "2024-02-01",
				},
			},
		},
		{
			name: "azure/missing api key",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					Endpoint:   "https://my.openai.azure.com",
					Deployment: "gpt-4o",
				},
			},
			wantErr: "AZURE_OPENAI_API_KEY",
		},
		{
			name: "azure/missing endpoint",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:     "key",
					Deployment: "gpt-4o",
				},
			},
			wantErr: "AZURE_OPENAI_ENDPOINT",
		},
		{
			name: "azure/missing deployment",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:   "key",
					Endpoint: "https://my.openai.azure.com",
				},
			},
			wantErr: "AZURE_OPENAI_DEPLOYMENT",
		},

		// ── Azure Codex ──────────────────────────────────────────────────────
		{
			name: "azure-codex/valid without deployment",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:     "key",
					Endpoint:   "https://my.openai.azure.com",
					APIVersion: "2025-04-01-preview",
					Codex: &Codex{
						Enabled:          true,
						Model:            "gpt-5.2-codex",
						DefaultMaxTokens: CodexDefaultMaxTokens,
					},
				},
			},
		},
		{
			name: "azure-codex/valid with all fields",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:     "key",
					Endpoint:   "https://my.openai.azure.com",
					APIVersion: "2025-04-01-preview",
					Codex: &Codex{
						Enabled:              true,
						Model:                "gpt-5.2-codex",
						DefaultMaxTokens:     32768,
						DefaultContext:       150000,
						HardMaxTokens:        65536,
						HardMaxContextTokens: 300000,
					},
				},
			},
		},
		{
			name: "azure-codex/disabled requires deployment",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:   "key",
					Endpoint: "https://my.openai.azure.com",
					Codex:    &Codex{Enabled: false},
				},
			},
			wantErr: "AZURE_OPENAI_DEPLOYMENT",
		},
		{
			name: "azure-codex/nil codex requires deployment",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey:   "key",
					Endpoint: "https://my.openai.azure.com",
					Codex:    nil,
				},
			},
			wantErr: "AZURE_OPENAI_DEPLOYMENT",
		},
		{
			name: "azure-codex/missing api key",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					Endpoint: "https://my.openai.azure.com",
					Codex:    &Codex{Enabled: true, Model: "gpt-5.2-codex"},
				},
			},
			wantErr: "AZURE_OPENAI_API_KEY",
		},
		{
			name: "azure-codex/missing endpoint",
			cfg: Config{
				Backend: BackendAzure,
				AzureOpenAI: ProviderAzureOpenAI{
					APIKey: "key",
					Codex:  &Codex{Enabled: true, Model: "gpt-5.2-codex"},
				},
			},
			wantErr: "AZURE_OPENAI_ENDPOINT",
		},

		// ── Bedrock ───────────────────────────────────────────────────────────
		{
			name: "bedrock/valid",
			cfg: Config{
				Backend: BackendBedrock,
				Bedrock: ProviderBedrock{AWSRegion: "us-east-1", ModelID: "anthropic.claude-3"},
			},
		},
		{
			name:    "bedrock/missing model id",
			cfg:     Config{Backend: BackendBedrock, Bedrock: ProviderBedrock{AWSRegion: "us-east-1"}},
			wantErr: "BEDROCK_MODEL_ID",
		},
		{
			name:    "bedrock/missing region",
			cfg:     Config{Backend: BackendBedrock, Bedrock: ProviderBedrock{ModelID: "anthropic.claude-3"}},
			wantErr: "AWS_REGION",
		},

		// ── Gemini ────────────────────────────────────────────────────────────
		{
			name: "gemini/valid",
			cfg: Config{
				Backend: BackendGemini,
				Gemini:  ProviderGemini{APIKey: "AIza-test", Model: "gemini-1.5-pro"},
			},
		},
		{
			name:    "gemini/missing api key",
			cfg:     Config{Backend: BackendGemini, Gemini: ProviderGemini{Model: "gemini-1.5-pro"}},
			wantErr: "GOOGLE_API_KEY",
		},
		{
			name:    "gemini/missing model",
			cfg:     Config{Backend: BackendGemini, Gemini: ProviderGemini{APIKey: "AIza-test"}},
			wantErr: "GEMINI_MODEL",
		},

		// ── Unknown backend ───────────────────────────────────────────────────
		{
			name:    "unknown backend",
			cfg:     Config{Backend: "unknown"},
			wantErr: "unknown backend",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestIsCodexEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  ProviderAzureOpenAI
		want bool
	}{
		{
			name: "codex enabled",
			cfg:  ProviderAzureOpenAI{Codex: &Codex{Enabled: true}},
			want: true,
		},
		{
			name: "codex disabled",
			cfg:  ProviderAzureOpenAI{Codex: &Codex{Enabled: false}},
			want: false,
		},
		{
			name: "codex nil",
			cfg:  ProviderAzureOpenAI{Codex: nil},
			want: false,
		},
		{
			name: "empty struct",
			cfg:  ProviderAzureOpenAI{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.cfg.isCodexEnabled()
			if got != tc.want {
				t.Errorf("isCodexEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCodexConstants(t *testing.T) {
	t.Parallel()

	// Verify constants have sensible values
	tests := []struct {
		name     string
		got      int
		wantMin  int
		wantMax  int
		wantDesc string
	}{
		{
			name:     "CodexDefaultMaxTokens",
			got:      CodexDefaultMaxTokens,
			wantMin:  16384,
			wantMax:  65536,
			wantDesc: "default output tokens should be 16K-64K",
		},
		{
			name:     "CodexDefaultContextTokens",
			got:      CodexDefaultContextTokens,
			wantMin:  100000,
			wantMax:  200000,
			wantDesc: "default context should be 100K-200K",
		},
		{
			name:     "CodexHardMaxTokens",
			got:      CodexHardMaxTokens,
			wantMin:  32768,
			wantMax:  131072,
			wantDesc: "hard max output should be 32K-128K",
		},
		{
			name:     "CodexHardMaxContextTokens",
			got:      CodexHardMaxContextTokens,
			wantMin:  200000,
			wantMax:  400000,
			wantDesc: "hard max context should be 200K-400K",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got < tc.wantMin || tc.got > tc.wantMax {
				t.Errorf("%s = %d, want between %d and %d (%s)",
					tc.name, tc.got, tc.wantMin, tc.wantMax, tc.wantDesc)
			}
		})
	}

	// Verify relationships between constants
	if CodexDefaultMaxTokens > CodexHardMaxTokens {
		t.Errorf("CodexDefaultMaxTokens (%d) should be <= CodexHardMaxTokens (%d)",
			CodexDefaultMaxTokens, CodexHardMaxTokens)
	}
	if CodexDefaultContextTokens > CodexHardMaxContextTokens {
		t.Errorf("CodexDefaultContextTokens (%d) should be <= CodexHardMaxContextTokens (%d)",
			CodexDefaultContextTokens, CodexHardMaxContextTokens)
	}
}

func TestIsAzureReasoningModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		deployment string
		want       bool
	}{
		// known o-series — should be detected
		{"o1", true},
		{"o1-preview", true},
		{"o1-mini", true},
		{"o3", true},
		{"o3-mini", true},
		{"o3-pro", true},
		{"o4-mini", true},
		{"O1-PREVIEW", true}, // case-insensitive
		{"O3-Mini", true},    // case-insensitive
		// codex-class — should be detected
		{"codex-mini", true},
		{"codex", true},
		{"gpt-5.2-codex", false}, // "codex" not at start — not matched by prefix rule
		// standard models — should NOT be detected
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4", false},
		{"gpt-4.1", false},
		{"gpt-35-turbo", false},
		{"my-custom-deployment", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.deployment, func(t *testing.T) {
			t.Parallel()
			got := isAzureReasoningModel(tc.deployment)
			if got != tc.want {
				t.Errorf("isAzureReasoningModel(%q) = %v, want %v", tc.deployment, got, tc.want)
			}
		})
	}
}
