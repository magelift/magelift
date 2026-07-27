# External Integrations

**Analysis Date:** 2026-07-27

## APIs & External Services

**Cloud Infrastructure (Pulumi-managed):**
- AWS - `github.com/pulumi/pulumi-aws/sdk/v7`; certified target (ECS Fargate, Route 53, CloudFront, WAF, ALB, S3) — `internal/cloud/aws/stack`
- GCP - `github.com/pulumi/pulumi-gcp/sdk/v9`; experimental (GKE Autopilot, Cloud SQL, Memorystore) — `internal/cloud/gcp`
- OVH - `github.com/ovh/pulumi-ovh/sdk/v2`; experimental (Managed Kubernetes/MKS) — `internal/cloud/ovh/stack/ops.go`
- Scaleway - `github.com/pulumiverse/pulumi-scaleway/sdk`; experimental (Kapsule) — `internal/cloud/scaleway/stack/ops.go`
- Kubernetes (generic) - `github.com/pulumi/pulumi-kubernetes/sdk/v4` + `k8s.io/client-go` for GKE/EKS/MKS/Kapsule day-2 workloads — `internal/cloud/kube`

**Cloud SDKs (direct API, outside Pulumi):**
- AWS SDK v2 - `internal/cloud/aws/ops/day2.go`, `internal/cloud/aws/runtime/runtime.go`, `internal/cloud/aws/cost/`, `internal/cloud/aws/eksops/ops.go`
  - Services used: ECS, S3, Secrets Manager, SSM Parameter Store, KMS, STS, IAM, CloudWatch Logs, Pricing
  - Auth: standard AWS credential chain via `aws-sdk-go-v2/config`; `MAGELIFT_AWS_ENDPOINT_URL` overrides endpoint for local emulator testing (Floci)
- GCP client libraries - `google.golang.org/api`, `cloud.google.com/go/{container,secretmanager,storage}` - `internal/cloud/gcp/ops/day2.go`
  - Auth: `golang.org/x/oauth2` / Application Default Credentials

**Artifact Signing:**
- Cosign (external CLI, invoked via `os/exec`) - `internal/cosign/cosign.go`
  - Verifies registry-qualified `sha256:` image digests against Sigstore/OIDC certificate identity and issuer
  - Not a Go SDK dependency; requires `cosign` binary on PATH

**Container/Compose runtime:**
- Local Docker daemon via generated Compose files - `internal/localdev/compose.go`, `internal/containerrunner/`
- BuildKit / Docker Buildx Bake for image builds - `docker-bake.hcl`, `images/{php-runtime,frankenphp-classic,varnish}/`

## Data Storage

**Databases:**
- MySQL 8.4 (`mysql:8.4@sha256:...`) - local dev default DB, overridable via `MAGELIFT_LOCAL_DATABASE_IMAGE` — `internal/localdev/compose.go`
- AWS Aurora/RDS MySQL - production catalog option, `aWSCatalogAurora` in `schema/magelift.schema.json`
- GCP Cloud SQL - experimental production database — `docs/gcp-experimental.md`

**Caching:**
- Valkey 8.1 (`valkey/valkey:8.1@sha256:...`) - local dev default cache — `internal/localdev/compose.go`

**Search:**
- OpenSearch 3 (`opensearchproject/opensearch:3@sha256:...`) - optional local service (`--service app`) — `internal/localdev/compose.go`

**Message Queue:**
- RabbitMQ 4.2-management (`rabbitmq:4.2-management@sha256:...`) - optional local service — `internal/localdev/compose.go`

**File Storage:**
- AWS S3 - production media path (certified AWS target) and Pulumi/Floci state bucket
- GCP Cloud Storage - `cloud.google.com/go/storage` for GCP media/artifacts

**Secrets:**
- References resolved at runtime, never plaintext in config (`internal/secretref/secretref.go`)
  - Supported schemes: `aws-secrets-manager://`, `ssm://`, `gcp-secret-manager://`
  - AWS Secrets Manager and SSM Parameter Store via `aws-sdk-go-v2/service/{secretsmanager,ssm}`
  - GCP Secret Manager via `cloud.google.com/go/secretmanager`
  - AWS KMS for key operations (`aws-sdk-go-v2/service/kms`)

## Authentication & Identity

**Cloud provider auth:**
- AWS - default credential chain (env vars, shared config, SSO/STS) via `aws-sdk-go-v2/config`, `aws-sdk-go-v2/credentials`, `aws-sdk-go-v2/service/sts`
- GCP - OAuth2/ADC via `golang.org/x/oauth2`
- GitHub Actions - OIDC role separation for CI deploys per ADR 0006 (`docs/adr/` — see `internal/cloud/aws` roles), referenced in `.github/workflows/ci.yml`

**Artifact provenance:**
- Sigstore/Cosign keyless signing - certificate identity + OIDC issuer verification (`internal/cosign/cosign.go`)

## Monitoring & Observability

**Logs:**
- AWS CloudWatch Logs - `aws-sdk-go-v2/service/cloudwatchlogs`, `internal/cloud/aws/eksops/ops.go`, `internal/cloud/aws/ops/day2.go`

**Cost:**
- AWS Pricing API - `aws-sdk-go-v2/service/pricing`, `internal/cloud/aws/cost/`, `internal/cli/cost.go`, `internal/platform/cost.go`

**Error Tracking:**
- None detected (no Sentry/Datadog/etc. SDK in `go.mod`)

## CI/CD & Deployment

**Hosting:**
- User-owned cloud accounts (AWS/GCP/OVH/Scaleway) - MageLift is not a hosting service itself

**CI Pipeline:**
- GitHub Actions - `.github/workflows/ci.yml`
  - `actions/setup-go` v7, `golangci/golangci-lint-action` v9.3.0
  - Path-filtered jobs (`needs.changes.outputs.go`)
  - `go test -race ./...` and `go test -race -tags=floci ./tests/floci` (Floci AWS emulator via `docker-compose.floci.yml`)
  - License compliance: `go-licenses/v2` with `--disallowed_types=forbidden,unknown`
  - Workflow linting: `actionlint`
- Release: GoReleaser v2.17.0 (`.goreleaser.yaml`) with SBOM generation and artifact signing

**Release process:**
- `release-please` for changelog/versioning (`release-please-config.json`, `.release-please-manifest.json`, `CHANGELOG.md`)

## Environment Configuration

**Required/observed env vars:**
- `PULUMI_BACKEND_URL` - Pulumi state backend (DIY/local)
- `MAGELIFT_AWS_ENDPOINT_URL` - AWS endpoint override for loopback emulators (Floci)
- `MAGELIFT_LOCAL_ADMIN_PASSWORD` - local Magento admin password (written only to `.magelift/local.env`)
- `MAGELIFT_LOCAL_DATABASE_IMAGE`, `MAGELIFT_LOCAL_CACHE_IMAGE`, `MAGELIFT_LOCAL_SEARCH_IMAGE`, `MAGELIFT_LOCAL_QUEUE_IMAGE` - override digest-pinned local dev images
- `MAGELIFT_LOCAL_HTTPS_PORT` - local Caddy HTTPS port (default 8443)

**Secrets location:**
- Cloud production secrets: resolved via `internal/secretref` at runtime from AWS Secrets Manager / SSM / GCP Secret Manager — never stored in `magelift.yaml`
- Local dev secrets: `.magelift/local.env` (gitignored, mode 0600)

## Webhooks & Callbacks

**Incoming:**
- None detected — MageLift is a CLI/IaC tool, not a running service with HTTP endpoints of its own

**Outgoing:**
- None detected beyond cloud provider API calls (AWS/GCP/OVH/Scaleway/Kubernetes) and the `cosign` CLI invocation

---

*Integration audit: 2026-07-27*
