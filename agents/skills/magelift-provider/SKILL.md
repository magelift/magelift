---
name: magelift-provider
description: >-
  Add or change a MageLift cloud provider adapter. Use when creating
  internal/cloud/<provider>/ packages, wiring StackModule registration, or
  updating capability matrix / experimental docs for a new cloud.
---

# Adding a MageLift provider

Read `docs/adding-a-provider.md` and ADRs 0002, 0004, 0007, 0008 before coding.

## Boundary

- Magento-facing code stays in `sdk/v1`, `internal/platform`, `internal/deploy`,
  `internal/config`
- VPC, DB, runtime, and Pulumi components stay under `internal/cloud/<provider>/`
- Do not share Pulumi Network/Database components behind a provider switch

## Registration seams

| Seam | Role |
| --- | --- |
| `platform.ModuleRegistry` | What the CLI uses for preview/deploy/destroy/outputs |
| `internal/infra.Registry` | Target/capability index for tests; alone does not wire CLI |

A PR that only calls `infra.RegisterTarget` will not appear in `magelift deploy`.

## Checklist (copy GCP)

1. `internal/cloud/<p>/target/`: IDs, Validate, optional infra register
2. `internal/cloud/<p>/stack/`: Spec, PlanFromConfig, Pulumi Program, StackModule
3. Capability packages as needed; keep them typed and small
4. Optional day-2 ports (`HasOps`, `HasBootstrap`, …) returning `ErrNotSupported`
   until ready (ADR 0009)
5. `internal/config`: provider block + `make generate` for schema
6. `cmd/magelift`: `RegisterModule(...)` in the production binary
7. Mock Pulumi graph tests; docs for experimental vs acceptance
8. Capability matrix row; never claim certified without acceptance evidence

## Honesty

Experimental providers are fine. Selling them as production-supported is not.
