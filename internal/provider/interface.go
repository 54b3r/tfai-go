// Package provider defines the ModelProvider interface and factory for
// selecting and constructing LLM backend implementations at runtime.
// Supported backends: Ollama, OpenAI, Azure OpenAI, AWS Bedrock, Google Gemini.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/model"
)

// Backend enumerates the supported LLM inference providers.
type Backend string

const (
	// BackendOllama selects a locally running Ollama instance.
	BackendOllama Backend = "ollama"
	// BackendOpenAI selects the OpenAI API.
	BackendOpenAI Backend = "openai"
	// BackendAzure selects Azure OpenAI Service.
	BackendAzure Backend = "azure"
	// BackendBedrock selects AWS Bedrock.
	BackendBedrock Backend = "bedrock"
	// BackendGemini selects Google Gemini via Vertex AI or AI Studio.
	BackendGemini Backend = "gemini"
)

type HealthCheckConfig interface {
	GetURL() string
	GetProviderType() Backend
	HealthCheck(ctx context.Context) error
}

// Concrete type that satisfies HealthCheckConfig
type healthCheckCfg struct {
	url          string
	providerType Backend
	apiKey       string
	check        func(ctx context.Context, url string, apiKey string) error
}

func (h *healthCheckCfg) GetURL() string                        { return h.url }
func (h *healthCheckCfg) GetProviderType() Backend              { return h.providerType }
func (h *healthCheckCfg) HealthCheck(ctx context.Context) error { return h.check(ctx, h.url, h.apiKey) }

// doHealthGet sends a GET request and returns nil on 2xx, error otherwise.
func doHealthGet(ctx context.Context, url string, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check: HTTP %d", resp.StatusCode)
	}
	return nil
}

// No auth — just GET and check 2xx
func httpGetCheck(ctx context.Context, url, _ string) error {
	return doHealthGet(ctx, url, nil)
}

// Bearer token auth
func bearerAuthCheck(ctx context.Context, url, apiKey string) error {
	return doHealthGet(ctx, url, map[string]string{"Authorization": "Bearer " + apiKey})
}

// Azure api-key header
func azureAPIKeyCheck(ctx context.Context, url, apiKey string) error {
	return doHealthGet(ctx, url, map[string]string{"api-key": apiKey})
}

// Config holds all provider-level configuration resolved from environment
// variables. Each provider uses its own native credential env vars rather
// than a homogenised MODEL_API_KEY abstraction.
type Config struct {
	// Backend identifies which inference provider to use (MODEL_PROVIDER).
	Backend Backend

	// Ollama holds config for a locally running Ollama instance.
	Ollama ProviderOllama

	// OpenAI holds config for the OpenAI API.
	OpenAI ProviderOpenAI

	// AzureOpenAI holds config for Azure OpenAI Service.
	AzureOpenAI ProviderAzureOpenAI

	// Bedrock holds config for AWS Bedrock. Credentials are resolved via
	// the standard AWS SDK credential chain — no key fields needed here.
	Bedrock ProviderBedrock

	// Gemini holds config for Google Gemini (AI Studio or Vertex AI).
	Gemini ProviderGemini

	// Tuning holds shared generation parameters applied to all backends.
	Tuning SharedTuning
}

// ProviderOllama holds configuration for a locally running Ollama instance.
type ProviderOllama struct {
	// Host is the Ollama server base URL (OLLAMA_HOST).
	Host string
	// Model is the Ollama model name to use (OLLAMA_MODEL).
	Model string
}

// NewHealthCheckConfig constructs a zero-cost HealthCheckConfig for the given
// backend. The returned config encapsulates the provider's metadata endpoint
// URL, credentials, and HTTP check function so callers only need to call
// HealthCheck(ctx).
func NewHealthCheckConfig(b Backend, cfg *Config) HealthCheckConfig {
	switch b {
	case BackendOllama:
		return &healthCheckCfg{
			url:          cfg.Ollama.Host + "/api/tags",
			providerType: b,
			check:        httpGetCheck,
		}
	case BackendOpenAI:
		return &healthCheckCfg{
			url:          "https://api.openai.com/v1/models",
			providerType: b,
			apiKey:       cfg.OpenAI.APIKey,
			check:        bearerAuthCheck,
		}
	case BackendAzure:
		return &healthCheckCfg{
			url:          cfg.AzureOpenAI.Endpoint + "/openai/models?api-version=" + cfg.AzureOpenAI.APIVersion,
			providerType: b,
			apiKey:       cfg.AzureOpenAI.APIKey,
			check:        azureAPIKeyCheck,
		}
	case BackendBedrock:
		return &healthCheckCfg{
			url:          "https://bedrock-runtime." + cfg.Bedrock.AWSRegion + ".amazonaws.com",
			providerType: b,
			check:        httpGetCheck,
		}
	case BackendGemini:
		return &healthCheckCfg{
			url:          "https://generativelanguage.googleapis.com/v1beta/models?key=" + cfg.Gemini.APIKey,
			providerType: b,
			check:        httpGetCheck,
		}
	default:
		return nil
	}
}

// ProviderOpenAI holds configuration for the OpenAI API.
type ProviderOpenAI struct {
	// APIKey is the OpenAI API key (OPENAI_API_KEY).
	APIKey string
	// Model is the OpenAI model ID (OPENAI_MODEL).
	Model string
}

// ProviderAzureOpenAI holds configuration for Azure OpenAI Service.
type ProviderAzureOpenAI struct {
	// APIKey is the Azure OpenAI API key (AZURE_OPENAI_API_KEY).
	APIKey string
	// Endpoint is the Azure OpenAI resource endpoint (AZURE_OPENAI_ENDPOINT).
	Endpoint string
	// Deployment is the Azure OpenAI deployment name (AZURE_OPENAI_DEPLOYMENT).
	Deployment string
	// APIVersion is the Azure OpenAI REST API version (AZURE_OPENAI_API_VERSION).
	APIVersion string
	// ReasoningOverride explicitly forces or disables reasoning-model mode,
	// overriding auto-detection. nil = auto-detect from deployment name.
	// Set AZURE_OPENAI_REASONING=true to force on, =false to force off.
	ReasoningOverride *bool
}

// ProviderBedrock holds configuration for AWS Bedrock.
// Credentials are resolved via the standard AWS SDK credential chain.
type ProviderBedrock struct {
	// AWSRegion is the AWS region (AWS_REGION).
	AWSRegion string
	// ModelID is the Bedrock model ID (BEDROCK_MODEL_ID).
	ModelID string
}

// ProviderGemini holds configuration for Google Gemini.
type ProviderGemini struct {
	// APIKey is the Google AI Studio API key (GOOGLE_API_KEY).
	APIKey string
	// Model is the Gemini model name (GEMINI_MODEL).
	Model string
}

// SharedTuning holds generation parameters shared across all backends.
type SharedTuning struct {
	// MaxTokens caps the number of tokens the model may generate per response.
	MaxTokens int
	// Temperature controls response randomness (0.0–1.0).
	Temperature float32
}

// Validate checks that all required fields for the selected backend are
// populated. It is called by New() before attempting to construct the model,
// so callers get a clear error at startup rather than on the first request.
func (c *Config) Validate() error {
	switch c.Backend {
	case BackendOllama:
		if c.Ollama.Model == "" {
			return fmt.Errorf("provider: %q requires OLLAMA_MODEL to be set", c.Backend)
		}
	case BackendOpenAI:
		if c.OpenAI.APIKey == "" {
			return fmt.Errorf("provider: %q requires OPENAI_API_KEY to be set", c.Backend)
		}
		if c.OpenAI.Model == "" {
			return fmt.Errorf("provider: %q requires OPENAI_MODEL to be set", c.Backend)
		}
	case BackendAzure:
		if c.AzureOpenAI.APIKey == "" {
			return fmt.Errorf("provider: %q requires AZURE_OPENAI_API_KEY to be set", c.Backend)
		}
		if c.AzureOpenAI.Endpoint == "" {
			return fmt.Errorf("provider: %q requires AZURE_OPENAI_ENDPOINT to be set", c.Backend)
		}
		if c.AzureOpenAI.Deployment == "" {
			return fmt.Errorf("provider: %q requires AZURE_OPENAI_DEPLOYMENT to be set", c.Backend)
		}
	case BackendBedrock:
		if c.Bedrock.ModelID == "" {
			return fmt.Errorf("provider: %q requires BEDROCK_MODEL_ID to be set", c.Backend)
		}
		if c.Bedrock.AWSRegion == "" {
			return fmt.Errorf("provider: %q requires AWS_REGION to be set", c.Backend)
		}
	case BackendGemini:
		if c.Gemini.APIKey == "" {
			return fmt.Errorf("provider: %q requires GOOGLE_API_KEY to be set", c.Backend)
		}
		if c.Gemini.Model == "" {
			return fmt.Errorf("provider: %q requires GEMINI_MODEL to be set", c.Backend)
		}
	default:
		return fmt.Errorf("provider: unknown backend %q — valid values: ollama, openai, azure, bedrock, gemini", c.Backend)
	}
	return nil
}

// Factory is the interface for constructing a ToolCallingChatModel from a Config.
// Implementations must be safe to call from multiple goroutines.
type Factory interface {
	// New constructs and returns a ready-to-use ToolCallingChatModel for the given config.
	New(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error)
}
