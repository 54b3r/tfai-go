package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/54b3r/tfai-go/internal/embedder"
	"github.com/54b3r/tfai-go/internal/ingestion"
	"github.com/54b3r/tfai-go/internal/rag"
)

// NewIngestCmd constructs the `tfai ingest` command, which runs the
// documentation ingestion pipeline to populate the RAG vector store.
func NewIngestCmd() *cobra.Command {
	var provider string
	var framework string
	var docType string
	var urls []string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest Terraform documentation into the RAG vector store",
		Long: `Fetch and index Terraform provider documentation into the Qdrant vector store.

Ingested documentation is used to provide accurate, provider-specific context
to the agent during queries, reducing hallucinations and improving code quality.

Required environment variables:
  QDRANT_HOST          Qdrant server hostname (default: localhost)
  QDRANT_PORT          Qdrant gRPC port (default: 6334)
  QDRANT_COLLECTION    Collection name (default: tfai-docs)
  QDRANT_API_KEY       Optional API key for authenticated clusters
  MODEL_PROVIDER       Embedding backend: ollama, openai, azure (default: ollama)
  EMBEDDING_*          Provider-specific overrides (see README)

Metadata flags (--provider, --framework, --doc-type) are optional. When omitted,
metadata is auto-inferred from the URL pattern (e.g. registry.terraform.io URLs
resolve provider and framework automatically). Explicit flags override inference.

Examples:
  tfai ingest --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster
  tfai ingest --url https://atmos.tools/core-concepts/stacks
  tfai ingest --provider aws --framework terraform --url https://example.com/custom-aws-doc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			log := slog.Default()

			if len(urls) == 0 {
				return fmt.Errorf("ingest: at least one --url is required")
			}

			if err := embedder.ValidateForRAG(log); err != nil {
				return fmt.Errorf("ingest: %w", err)
			}

			emb, err := embedder.NewFromEnv()
			if err != nil {
				return fmt.Errorf("ingest: failed to initialise embedder: %w", err)
			}
			log.Info("embedder initialised", slog.String("provider", getEnvOrDefault("EMBEDDING_PROVIDER", getEnvOrDefault("MODEL_PROVIDER", "ollama"))))

			qdrantHost := getEnvOrDefault("QDRANT_HOST", "localhost")
			qdrantPort := getEnvInt("QDRANT_PORT", 6334)
			collection := getEnvOrDefault("QDRANT_COLLECTION", "tfai-docs")
			embBackend := getEnvOrDefault("EMBEDDING_PROVIDER", getEnvOrDefault("MODEL_PROVIDER", "ollama"))
			vectorSize := uint64(embedder.DefaultDimensions(embBackend)) //nolint:gosec // dimensions are bounded

			store, err := rag.NewQdrantStore(ctx, &rag.QdrantConfig{
				Host:       qdrantHost,
				Port:       qdrantPort,
				Collection: collection,
				VectorSize: vectorSize,
				APIKey:     os.Getenv("QDRANT_API_KEY"),
				UseTLS:     os.Getenv("QDRANT_TLS") == "true",
			})
			if err != nil {
				return fmt.Errorf("ingest: failed to connect to Qdrant at %s:%d: %w", qdrantHost, qdrantPort, err)
			}
			defer store.Close()
			log.Info("qdrant store ready", slog.String("host", qdrantHost), slog.Int("port", qdrantPort), slog.String("collection", collection))

			pipeline, err := ingestion.NewPipeline(emb, store, nil)
			if err != nil {
				return fmt.Errorf("ingest: failed to create pipeline: %w", err)
			}

			providerSet := cmd.Flags().Changed("provider")
			frameworkSet := cmd.Flags().Changed("framework")
			docTypeSet := cmd.Flags().Changed("doc-type")

			sources := make([]ingestion.Source, 0, len(urls))
			for _, u := range urls {
				inferred := ingestion.InferMetadata(u)

				src := ingestion.Source{URL: u}
				if providerSet {
					src.Provider = provider
				} else {
					src.Provider = inferred.Provider
				}
				if frameworkSet {
					src.Framework = framework
				} else {
					src.Framework = inferred.Framework
				}
				if docTypeSet {
					src.DocType = docType
				} else {
					src.DocType = inferred.DocType
				}

				log.Info("source metadata",
					slog.String("url", u),
					slog.String("provider", src.Provider),
					slog.String("framework", src.Framework),
					slog.String("doc_type", src.DocType),
				)
				sources = append(sources, src)
			}

			log.Info("starting ingestion", slog.Int("sources", len(sources)))

			if err := pipeline.Ingest(ctx, sources, func(msg string) {
				log.Info(msg)
			}); err != nil {
				return fmt.Errorf("ingest: pipeline failed: %w", err)
			}

			log.Info("ingestion complete", slog.Int("sources", len(sources)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&provider, "provider", "p", "generic", "Cloud provider label (aws, azure, gcp, generic)")
	cmd.Flags().StringVarP(&framework, "framework", "f", "terraform", "IaC framework label (terraform, atmos, terragrunt, cdktf)")
	cmd.Flags().StringVarP(&docType, "doc-type", "d", "reference", "Documentation type (reference, tutorial, guide, api, changelog)")
	cmd.Flags().StringArrayVarP(&urls, "url", "u", nil, "Documentation URL to ingest (repeatable)")

	return cmd
}
