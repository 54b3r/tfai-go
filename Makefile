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
install-tools: ## Install local development tools (golangci-lint, goimports)
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest

# ── Build ─────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build the tfai binary natively
	$(GO) build $(GOFLAGS) -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/tfai

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

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

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
