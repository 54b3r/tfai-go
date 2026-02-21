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
