# TF-AI-Go Makefile — 3 Musketeers pattern
# All targets that require build tooling run inside Docker to ensure
# reproducible builds without requiring local Go/tool installation.
# Targets that are safe to run natively (lint, fmt) also work locally.

BINARY        := tfai
IMAGE         := tfai-go
IMAGE_TAG     ?= local
DOCKER_COMPOSE := docker compose
GO            := go
GOFLAGS       ?=

# Version info injected into the binary at build time via -ldflags.
# VERSION defaults to the most recent git tag; falls back to "dev".
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LD_FLAGS      := -s -w \
  -X github.com/54b3r/tfai-go/internal/version.Version=$(VERSION) \
  -X github.com/54b3r/tfai-go/internal/version.Commit=$(COMMIT) \
  -X github.com/54b3r/tfai-go/internal/version.BuildDate=$(BUILD_DATE)

# Source the .env file if it exists (for local native runs).
ifneq (,$(wildcard .env))
  include .env
  export
endif

.DEFAULT_GOAL := help

# ── Help ──────────────────────────────────────────────────────────────────────
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' | sort

# ── Dependencies ──────────────────────────────────────────────────────────────
.PHONY: deps
deps: ## Download and tidy Go module dependencies
	$(GO) mod download
	$(GO) mod tidy

.PHONY: install-tools
install-tools: ## Install local development tools (golangci-lint, goimports, govulncheck)
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

# ── Build ─────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build the tfai binary natively (version info injected via ldflags)
	$(GO) build $(GOFLAGS) -trimpath -ldflags="$(LD_FLAGS)" -o bin/$(BINARY) ./cmd/tfai

.PHONY: version
version: build ## Print the version of the locally built binary
	./bin/$(BINARY) version

.PHONY: build-docker
build-docker: ## Build the Docker image
	docker build -t $(IMAGE):$(IMAGE_TAG) .

# ── Run ───────────────────────────────────────────────────────────────────────
.PHONY: run
run: build ## Build and run tfai serve natively (requires .env)
	./bin/$(BINARY) serve

.PHONY: run-docker
run-docker: ## Run the full stack via docker compose (app + qdrant + langfuse)
	$(DOCKER_COMPOSE) up --build

.PHONY: up
up: ## Start supporting services only (qdrant + langfuse) — run tfai natively
	$(DOCKER_COMPOSE) up -d qdrant langfuse langfuse-db

.PHONY: down
down: ## Stop and remove all docker compose services
	$(DOCKER_COMPOSE) down

.PHONY: down-volumes
down-volumes: ## Stop services and remove all persistent volumes (destructive)
	$(DOCKER_COMPOSE) down -v

# ── Code quality ──────────────────────────────────────────────────────────────
.PHONY: fmt
fmt: ## Run gofmt and goimports on all Go source files
	gofmt -w .
	goimports -w .

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: check
check: fmt vet lint ## Run all code quality checks

# ── Test ──────────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run unit tests
	$(GO) test -race -count=1 ./...

.PHONY: test-verbose
test-verbose: ## Run unit tests with verbose output
	$(GO) test -race -count=1 -v ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# ── Lint ──────────────────────────────────────────────────────────────────────
.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint in PATH)
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --fix ./...

# ── Gate ──────────────────────────────────────────────────────────────────────
# Full pre-commit verification sequence. Must pass before any commit or PR.
# Steps: build → vet → lint → vulncheck → test → binary smoke (version + help)
.PHONY: gate
gate: ## Run full pre-commit gate (build + vet + lint + vulncheck + test + binary smoke)
	@echo "── gate: build ──────────────────────────────────────────"
	$(GO) build $(GOFLAGS) -trimpath -ldflags="$(LD_FLAGS)" -o bin/$(BINARY) ./cmd/tfai
	@echo "── gate: vet ────────────────────────────────────────────"
	$(GO) vet ./...
	@echo "── gate: lint ───────────────────────────────────────────"
	golangci-lint run ./...
	@echo "── gate: vulncheck ──────────────────────────────────────"
	govulncheck ./...
	@echo "── gate: test ───────────────────────────────────────────"
	$(GO) test -race -count=1 ./...
	@echo "── gate: binary smoke ───────────────────────────────────"
	./bin/$(BINARY) version
	./bin/$(BINARY) --help
	./bin/$(BINARY) serve --help
	@echo "── gate: PASS ───────────────────────────────────────────"

# ── Ingestion ─────────────────────────────────────────────────────────────────
.PHONY: ingest-aws
ingest-aws: build ## Ingest core AWS Terraform provider docs into Qdrant
	./bin/$(BINARY) ingest --provider aws \
	  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster \
	  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc \
	  --url https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role

.PHONY: ingest-azure
ingest-azure: build ## Ingest core Azure Terraform provider docs into Qdrant
	./bin/$(BINARY) ingest --provider azure \
	  --url https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/kubernetes_cluster \
	  --url https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/virtual_network

.PHONY: ingest-gcp
ingest-gcp: build ## Ingest core GCP Terraform provider docs into Qdrant
	./bin/$(BINARY) ingest --provider gcp \
	  --url https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster \
	  --url https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network

# ── Clean ─────────────────────────────────────────────────────────────────────
.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

.PHONY: clean-all
clean-all: clean down-volumes ## Remove build artifacts and all docker volumes
