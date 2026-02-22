package ingestion

import (
	"net/url"
	"strings"
)

// InferredMetadata holds the framework, provider, and doc type inferred from
// a documentation URL's structure. CLI flags take precedence over inferred
// values — this is the "best-effort" fallback when the user doesn't specify
// explicit metadata.
type InferredMetadata struct {
	// Framework is the IaC framework (terraform, atmos, terragrunt, cdktf).
	Framework string
	// Provider is the cloud provider label (aws, azure, gcp, generic).
	Provider string
	// DocType classifies the documentation kind (reference, tutorial, guide, api, changelog).
	DocType string
}

// registryProviderAliases maps the Terraform provider namespace used in
// registry URLs to our canonical short label.
var registryProviderAliases = map[string]string{
	"aws":         "aws",
	"azurerm":     "azure",
	"azuread":     "azure",
	"google":      "gcp",
	"google-beta": "gcp",
	"kubernetes":  "kubernetes",
	"helm":        "kubernetes",
	"random":      "generic",
	"null":        "generic",
	"local":       "generic",
	"tls":         "generic",
	"archive":     "generic",
	"template":    "generic",
	"http":        "generic",
}

// InferMetadata inspects the documentation source URL and returns best-effort
// metadata. If the URL doesn't match any known pattern the returned fields
// contain sensible defaults ("terraform", "generic", "reference").
//
// Supported URL patterns:
//
//	registry.terraform.io/providers/{org}/{provider}/...
//	atmos.tools/...
//	developer.hashicorp.com/terraform/tutorials/...
//	developer.hashicorp.com/terraform/language/...
//	terragrunt.gruntwork.io/...
func InferMetadata(rawURL string) InferredMetadata {
	m := InferredMetadata{
		Framework: "terraform",
		Provider:  "generic",
		DocType:   "reference",
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return m
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	segments := trimSegments(path)

	switch {
	case host == "registry.terraform.io":
		inferRegistryTerraform(segments, &m)

	case host == "atmos.tools":
		m.Framework = "atmos"
		m.Provider = "atmos"
		inferAtmos(segments, &m)

	case host == "developer.hashicorp.com":
		inferHashiCorpDeveloper(segments, &m)

	case host == "terragrunt.gruntwork.io":
		m.Framework = "terragrunt"
		m.DocType = "reference"

	case strings.HasSuffix(host, "cdktf.io") || strings.Contains(path, "cdktf"):
		m.Framework = "cdktf"
		m.DocType = "reference"
	}

	return m
}

// inferRegistryTerraform handles registry.terraform.io/providers/{org}/{name}/...
func inferRegistryTerraform(segments []string, m *InferredMetadata) {
	// Expected path: providers / {org} / {name} / latest / docs / ...
	if len(segments) < 3 || segments[0] != "providers" {
		return
	}

	providerName := segments[2] // e.g. "aws", "azurerm", "google"
	if alias, ok := registryProviderAliases[providerName]; ok {
		m.Provider = alias
	} else {
		m.Provider = providerName
	}

	// Detect doc type from deeper path segments.
	for _, seg := range segments {
		switch seg {
		case "guides":
			m.DocType = "guide"
			return
		case "resources", "data-sources":
			m.DocType = "reference"
			return
		}
	}
}

// inferAtmos handles atmos.tools/...
func inferAtmos(segments []string, m *InferredMetadata) {
	if len(segments) == 0 {
		return
	}
	switch segments[0] {
	case "quick-start", "getting-started":
		m.DocType = "tutorial"
	case "core-concepts", "cli", "design-patterns":
		m.DocType = "reference"
	case "integrations":
		m.DocType = "guide"
	case "cheatsheets":
		m.DocType = "reference"
	}
}

// inferHashiCorpDeveloper handles developer.hashicorp.com/terraform/...
func inferHashiCorpDeveloper(segments []string, m *InferredMetadata) {
	if len(segments) < 2 || segments[0] != "terraform" {
		return
	}
	switch segments[1] {
	case "tutorials":
		m.DocType = "tutorial"
	case "language", "cli", "internals":
		m.DocType = "reference"
	case "plugin":
		m.DocType = "api"
	}
}

// trimSegments splits a URL path into non-empty lowercase segments.
func trimSegments(path string) []string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
