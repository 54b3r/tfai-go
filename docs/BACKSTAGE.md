# Backstage Integration Guide

This guide covers how to register TF-AI in a Backstage software catalog and use
the scaffolder template to spin up pre-configured instances for your teams.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Deploying Backstage](#deploying-backstage)
3. [Registering the TF-AI Catalog Entity](#registering-the-tf-ai-catalog-entity)
4. [Installing the Scaffolder Template](#installing-the-scaffolder-template)
5. [Using the Template](#using-the-template)
6. [Validating the Integration](#validating-the-integration)
7. [Customising the Template](#customising-the-template)
8. [Troubleshooting](#troubleshooting)

---

## Prerequisites

- **Backstage instance** — either an existing deployment or a fresh one (setup below)
- **Node.js** ≥ 18 and **yarn** (for Backstage development)
- **GitHub App or token** — for the scaffolder to create repositories
- **Docker** and **Docker Compose** — for running TF-AI instances

---

## Deploying Backstage

If you don't have an existing Backstage instance, follow these steps to create one.

### 1. Create a new Backstage app

```bash
npx @backstage/create-app@latest
cd my-backstage-app
```

### 2. Configure GitHub integration

Edit `app-config.yaml` to add your GitHub credentials:

```yaml
integrations:
  github:
    - host: github.com
      # Option A: Personal Access Token
      token: ${GITHUB_TOKEN}
      # Option B: GitHub App (recommended for production)
      # apps:
      #   - appId: ${GITHUB_APP_ID}
      #     privateKey: ${GITHUB_APP_PRIVATE_KEY}
      #     webhookSecret: ${GITHUB_WEBHOOK_SECRET}
      #     clientId: ${GITHUB_APP_CLIENT_ID}
      #     clientSecret: ${GITHUB_APP_CLIENT_SECRET}
```

### 3. Start Backstage

```bash
yarn dev
```

Backstage is now running at **http://localhost:3000** (frontend) and
**http://localhost:7007** (backend).

---

## Registering the TF-AI Catalog Entity

TF-AI ships with a `catalog-info.yaml` at the repo root that defines three
entities:

| Entity | Kind | Description |
|---|---|---|
| `tfai-go` | Component | The TF-AI service itself |
| `tfai-api` | API | OpenAPI definition for the REST/SSE endpoints |
| `qdrant` | Resource | Qdrant vector store dependency |

### Option A: Register via the Backstage UI

1. Navigate to **http://localhost:3000/catalog-import**
2. Enter the URL:
   ```
   https://github.com/54b3r/tfai-go/blob/main/catalog-info.yaml
   ```
3. Click **Analyze** → **Import**

### Option B: Register via `app-config.yaml`

Add the following to your Backstage `app-config.yaml`:

```yaml
catalog:
  locations:
    - type: url
      target: https://github.com/54b3r/tfai-go/blob/main/catalog-info.yaml
      rules:
        - allow: [Component, API, Resource]
```

Restart Backstage to pick up the new location.

### Verification

After import, navigate to the catalog and confirm:

- **tfai-go** appears as a `service` component with lifecycle `production`
- **tfai-api** appears as an API with an OpenAPI definition
- **qdrant** appears as a `database` resource
- The dependency graph shows `tfai-go → qdrant`

---

## Installing the Scaffolder Template

The scaffolder template lets teams create pre-configured TF-AI deployments.

### Option A: Register via the UI

1. Navigate to **http://localhost:3000/catalog-import**
2. Enter the URL:
   ```
   https://github.com/54b3r/tfai-go/blob/main/backstage/templates/tfai-instance/template.yaml
   ```
3. Click **Analyze** → **Import**

### Option B: Register via `app-config.yaml`

```yaml
catalog:
  locations:
    - type: url
      target: https://github.com/54b3r/tfai-go/blob/main/backstage/templates/tfai-instance/template.yaml
      rules:
        - allow: [Template]
```

### Verification

Navigate to **http://localhost:3000/create** — the **"Deploy TF-AI Instance"**
template should appear in the list.

---

## Using the Template

### Step 1: Instance Configuration

| Field | Description | Default |
|---|---|---|
| **Instance Name** | Unique name (e.g. `tfai-team-platform`) | — |
| **Description** | Short purpose description | `TF-AI Terraform expert agent` |
| **Owner** | Team or user from the catalog | — |
| **Model Provider** | LLM backend (ollama, openai, azure, bedrock, gemini) | `ollama` |
| **Enable RAG** | Deploy Qdrant alongside TF-AI | `true` |

### Step 2: Server Settings

| Field | Description | Default |
|---|---|---|
| **Server Port** | HTTP port for TF-AI | `8080` |
| **Enable Auth** | Require Bearer token | `true` |
| **Enable Tracing** | Deploy Langfuse for LLM observability | `false` |

### Step 3: Repository Location

Select the GitHub organization and repository name for the new instance.

### What gets created

The scaffolder creates a new GitHub repository containing:

```
<instance-name>/
├── config.yaml            # Primary configuration (pre-filled from wizard)
├── catalog-info.yaml      # Backstage entity (auto-registered)
├── docker-compose.yaml    # TF-AI + Qdrant + optional Langfuse
├── .env.example           # Secrets template (API keys only)
├── Makefile               # up, down, run, logs, ingest-all
└── README.md              # Instance-specific docs
```

The new component is automatically registered in the Backstage catalog.

---

## Validating the Integration

After creating an instance via the scaffolder, verify end-to-end:

### 1. Catalog registration

```bash
# The new component should appear in the catalog
open http://localhost:3000/catalog
```

### 2. Clone and configure

```bash
git clone <new-repo-url>
cd <instance-name>

# config.yaml is pre-configured — review and adjust as needed
# Add secrets (API keys only)
cp .env.example .env
# Edit .env — uncomment and set API keys for your provider
```

### 3. Start the instance

```bash
make up    # Start Qdrant (+ Langfuse if enabled)
make run   # Start TF-AI
```

### 4. Health check

```bash
curl http://localhost:8080/api/health
# {"status":"ok"}

curl http://localhost:8080/api/ready
# {"ready":true,"checks":[...]}
```

### 5. Ingest documents (if RAG enabled)

```bash
make ingest-all
```

### 6. Test the agent

```bash
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"How do I create an EKS cluster with Terraform?"}'
```

---

## Customising the Template

### Adding new parameters

Edit `backstage/templates/tfai-instance/template.yaml` → `spec.parameters`
to add new form fields. The values are available in skeleton files as
`${{ values.fieldName }}`.

### Modifying skeleton files

Edit files in `backstage/templates/tfai-instance/skeleton/`. These are
Nunjucks templates — use `${{ values.* }}` for interpolation and
`{%- if values.* %}` for conditionals.

### Adding new skeleton files

Any file added to the `skeleton/` directory will be included in scaffolded
repositories. Use template syntax for dynamic content.

### Testing changes

Use the Backstage **Template Editor** at `/create/edit` to test template
changes without creating real repositories.

---

## Troubleshooting

### Template not appearing in /create

- Verify the template URL is registered in `catalog.locations`
- Check Backstage backend logs for import errors
- Ensure the template YAML is valid (`apiVersion: scaffolder.backstage.io/v1beta3`)

### Scaffolder fails to create repository

- Verify GitHub integration is configured with sufficient permissions
- The token/app needs `repo` scope (or `contents:write` + `metadata:read` for fine-grained)
- Check Backstage scaffolder logs: `yarn backstage-cli backend:dev`

### Catalog entity not found after scaffolding

- The `catalog:register` step requires the `catalog-info.yaml` to be committed
  to the new repository's default branch
- Verify the `catalogInfoPath` in the template matches the actual file path

### Skeleton YAML lint errors in IDE

- The `{%- if %}` / `{%- endif %}` directives in skeleton `.yaml` files are
  **Nunjucks template syntax**, not YAML. They are processed by Backstage's
  scaffolder before YAML parsing. IDE YAML linters will flag these — they are
  safe to ignore.

### Health check fails after deployment

- Ensure the `.env` file has the correct `MODEL_PROVIDER` and credentials
- Check `QDRANT_HOST` is set to `qdrant` (the Docker Compose service name)
- Run `docker compose logs tfai` to see the audit log and error details
