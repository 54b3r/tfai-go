package ingestion

import "testing"

func TestInferMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		framework string
		provider  string
		docType   string
	}{
		// ── Terraform Registry: AWS ──────────────────────────────────────
		{
			name:      "registry aws resource",
			url:       "https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster",
			framework: "terraform",
			provider:  "aws",
			docType:   "reference",
		},
		{
			name:      "registry aws data source",
			url:       "https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/vpc",
			framework: "terraform",
			provider:  "aws",
			docType:   "reference",
		},
		{
			name:      "registry aws guide",
			url:       "https://registry.terraform.io/providers/hashicorp/aws/latest/docs/guides/version-4-upgrade",
			framework: "terraform",
			provider:  "aws",
			docType:   "guide",
		},
		// ── Terraform Registry: Azure ────────────────────────────────────
		{
			name:      "registry azurerm resource",
			url:       "https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/kubernetes_cluster",
			framework: "terraform",
			provider:  "azure",
			docType:   "reference",
		},
		{
			name:      "registry azuread resource",
			url:       "https://registry.terraform.io/providers/hashicorp/azuread/latest/docs/resources/application",
			framework: "terraform",
			provider:  "azure",
			docType:   "reference",
		},
		// ── Terraform Registry: GCP ─────────────────────────────────────
		{
			name:      "registry google resource",
			url:       "https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster",
			framework: "terraform",
			provider:  "gcp",
			docType:   "reference",
		},
		{
			name:      "registry google-beta resource",
			url:       "https://registry.terraform.io/providers/hashicorp/google-beta/latest/docs/resources/compute_instance",
			framework: "terraform",
			provider:  "gcp",
			docType:   "reference",
		},
		// ── Terraform Registry: other providers ─────────────────────────
		{
			name:      "registry kubernetes resource",
			url:       "https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/deployment",
			framework: "terraform",
			provider:  "kubernetes",
			docType:   "reference",
		},
		{
			name:      "registry unknown provider",
			url:       "https://registry.terraform.io/providers/someorg/custom/latest/docs/resources/thing",
			framework: "terraform",
			provider:  "custom",
			docType:   "reference",
		},
		// ── Atmos ───────────────────────────────────────────────────────
		{
			name:      "atmos core concepts",
			url:       "https://atmos.tools/core-concepts/stacks",
			framework: "atmos",
			provider:  "atmos",
			docType:   "reference",
		},
		{
			name:      "atmos quick start",
			url:       "https://atmos.tools/quick-start/configure-cli",
			framework: "atmos",
			provider:  "atmos",
			docType:   "tutorial",
		},
		{
			name:      "atmos cli commands",
			url:       "https://atmos.tools/cli/commands/atmos-terraform",
			framework: "atmos",
			provider:  "atmos",
			docType:   "reference",
		},
		{
			name:      "atmos integrations",
			url:       "https://atmos.tools/integrations/github-actions",
			framework: "atmos",
			provider:  "atmos",
			docType:   "guide",
		},
		// ── HashiCorp Developer ──────────────────────────────────────────
		{
			name:      "hashicorp developer tutorial",
			url:       "https://developer.hashicorp.com/terraform/tutorials/aws-get-started/aws-build",
			framework: "terraform",
			provider:  "generic",
			docType:   "tutorial",
		},
		{
			name:      "hashicorp developer language ref",
			url:       "https://developer.hashicorp.com/terraform/language/values/variables",
			framework: "terraform",
			provider:  "generic",
			docType:   "reference",
		},
		{
			name:      "hashicorp developer cli ref",
			url:       "https://developer.hashicorp.com/terraform/cli/commands/plan",
			framework: "terraform",
			provider:  "generic",
			docType:   "reference",
		},
		{
			name:      "hashicorp developer plugin api",
			url:       "https://developer.hashicorp.com/terraform/plugin/framework",
			framework: "terraform",
			provider:  "generic",
			docType:   "api",
		},
		// ── Terragrunt ──────────────────────────────────────────────────
		{
			name:      "terragrunt docs",
			url:       "https://terragrunt.gruntwork.io/docs/features/keep-your-backend-dry/",
			framework: "terragrunt",
			provider:  "generic",
			docType:   "reference",
		},
		// ── Fallback / unknown ──────────────────────────────────────────
		{
			name:      "completely unknown URL",
			url:       "https://example.com/some/random/page",
			framework: "terraform",
			provider:  "generic",
			docType:   "reference",
		},
		{
			name:      "malformed URL",
			url:       "://not-a-url",
			framework: "terraform",
			provider:  "generic",
			docType:   "reference",
		},
		{
			name:      "empty string",
			url:       "",
			framework: "terraform",
			provider:  "generic",
			docType:   "reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InferMetadata(tt.url)

			if got.Framework != tt.framework {
				t.Errorf("Framework: got %q, want %q", got.Framework, tt.framework)
			}
			if got.Provider != tt.provider {
				t.Errorf("Provider: got %q, want %q", got.Provider, tt.provider)
			}
			if got.DocType != tt.docType {
				t.Errorf("DocType: got %q, want %q", got.DocType, tt.docType)
			}
		})
	}
}
