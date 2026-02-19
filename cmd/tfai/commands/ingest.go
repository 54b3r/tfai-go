package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/ingestion"
)

// NewIngestCmd constructs the `tfai ingest` command, which runs the
// documentation ingestion pipeline to populate the RAG vector store.
func NewIngestCmd() *cobra.Command {
	var provider string
	var urls []string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest Terraform documentation into the RAG vector store",
		Long: `Fetch and index Terraform provider documentation into the Qdrant vector store.

Ingested documentation is used to provide accurate, provider-specific context
to the agent during queries, reducing hallucinations and improving code quality.

Examples:
  tfai ingest --provider aws --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster
  tfai ingest --provider azure --url https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/kubernetes_cluster
  tfai ingest --provider gcp --url https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if len(urls) == 0 {
				return fmt.Errorf("ingest: at least one --url is required")
			}

			// TODO: wire up real embedder + Qdrant store from env config.
			// For now this validates the pipeline construction path.
			sources := make([]ingestion.Source, 0, len(urls))
			for _, u := range urls {
				sources = append(sources, ingestion.Source{
					URL:          u,
					Provider:     provider,
					ResourceType: "",
				})
			}

			fmt.Printf("ingestion pipeline: %d source(s) queued for provider %q\n", len(sources), provider)
			fmt.Println("note: embedder and vector store must be configured via env vars (see README)")
			fmt.Println("TODO: wire QDRANT_HOST, QDRANT_PORT, and embedding model env vars")

			_ = ctx
			return nil
		},
	}

	cmd.Flags().StringVarP(&provider, "provider", "p", "generic", "Cloud provider label (aws, azure, gcp, generic)")
	cmd.Flags().StringArrayVarP(&urls, "url", "u", nil, "Documentation URL to ingest (repeatable)")

	return cmd
}
