# syntax=docker/dockerfile:1

# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependency downloads separately from source compilation.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/tfai ./cmd/tfai

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

# Install terraform binary for plan/state tools.
ARG TERRAFORM_VERSION=1.9.8
RUN apk add --no-cache curl unzip && \
    curl -fsSL "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip" \
    -o /tmp/terraform.zip && \
    unzip /tmp/terraform.zip -d /usr/local/bin && \
    rm /tmp/terraform.zip && \
    apk del curl unzip

WORKDIR /app

COPY --from=builder /out/tfai /usr/local/bin/tfai
COPY ui/ ./ui/

# Non-root user for security.
RUN addgroup -S tfai && adduser -S tfai -G tfai
USER tfai

EXPOSE 8080

ENTRYPOINT ["tfai"]
CMD ["serve"]
