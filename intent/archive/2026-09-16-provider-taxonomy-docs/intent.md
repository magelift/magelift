---
status: done
slug: provider-taxonomy-docs
---

# Intent: provider taxonomy and docs

## Problem

Providers live under three roots plus config strings plus shell-only helpers, and shared recovery ports are misnamed as clouds. Newcomers cannot answer "where does provider X live", ownership is unclear, and per-provider release thinking has no folder foundation. SaaS and edge vendors (Fastly, New Relic, SES, Cloudflare) have no consistent home next to AWS/GCP/OVH/Scaleway.

## Evidence

- `internal/cloud/aws` (~35k LOC), `internal/cloud/gcp` (~21k), `internal/cloud/ovh` (~12k), `internal/cloud/scaleway` (~12k) hold IaaS adapters.
- `internal/external/fastly`, `internal/external/newrelic`, `internal/external/observability` (~13k total) plus `internal/edge` (736 LOC) hold SaaS/edge outside `internal/cloud/`.
- SES exists only as an `internal/config` `email.mode` string with credential validation in core schema; no adapter package.
- Cloudflare exists only in shell helpers (`tests/acceptance/cloudflare_dns_helper_test.sh`) and knowledge notes; no Go adapter.
- `internal/cloud/recovery`, `internal/cloud/resilience`, `internal/cloud/statearchive` are provider-neutral ports (e.g. `S3ObjectAPI` interface), not clouds, but read as a fifth/sixth/seventh cloud.
- `internal/cloud/kube` (~5.7k) is shared Magento-shaped K8s wiring importing `pulumi-kubernetes`, documented as allowed but coupling GKE/MKS/Kapsule to one k8s SDK version.
- `cmd/magelift-{aws,gcp,ovh,scaleway}` slim mains exist but ship nowhere (absent from `.goreleaser.yaml`).

## Proposed outcome

One documented provider-root decision exists and the tree matches it: every IaaS and SaaS provider has an explicit home or an explicit "not a provider" label with a reason. Shared ports no longer read as clouds. `docs/adding-a-provider.md`, ADR 0003, and the capability matrix agree on where a new provider goes. `make docs` and `make generate-check` stay green; no runtime behavior changes.

## Affected users and systems

Contributors adding or reviewing providers; `internal/cloud`, `internal/external`, `internal/edge`, `internal/config` docs; `docs/adding-a-provider.md`, `docs/adr/0003-*`, `docs/capability-matrix.md`; `cmd/magelift-*` slim mains; agent skills (`magelift-provider`, `magelift-contribute`) if paths change.

## Constraints

- Smallest move that clarifies; no behavior change; no new adapters in this change (a Cloudflare or SES Go adapter is a separate intent if wanted).
- Keep the v1 single-version promise; this is taxonomy, not independent releases.
- Human docs go through humanizer, then remove-ai-marks; update the ADR and the human page together when a topology rule changes.
- Do not break `platform.ModuleRegistry` wiring, `make generate` outputs, or Floci/mock test paths.
- Respect `eu-providers-experimental` scope notes for OVH/Scaleway; do not recertify by moving folders.

## Out of scope

- Multi-module split, `go.work`, per-provider versioning or releases.
- Subprocess extraction; SDK module extraction; core import-seam removal (covered by `sever-core-imports`).
- Implementing Cloudflare, SES, or new Fastly/New Relic capabilities.
- `internal/config` provider-struct decoupling (opaque `target.config` is later work).

## Open questions

- Target root: single `internal/providers/` for IaaS plus SaaS, keep `internal/cloud/` for IaaS with SaaS under a documented `internal/external/` rule, or another shape? Needs a decision before spec.
- Do `internal/cloud/{recovery,resilience,statearchive}` move to `internal/shared/` or `internal/platform/`, or stay with a rename and a doc note?
- Fate of `cmd/magelift-{aws,gcp,ovh,scaleway}`: wire into GoReleaser as slim per-cloud CLIs or delete as unreleased dead weight?
- Does `internal/cloud/kube` stay as the one blessed shared Pulumi helper with an explicit ADR carve-out, or get a stricter no-Pulumi rule?
