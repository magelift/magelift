---
name: magelift-provider
description: >-
  Add or change a MageLift cloud provider adapter. Use when creating
  internal/cloud/<provider>/ packages, wiring StackModule registration, or
  updating capability matrix / experimental docs for a new cloud.
version: 1.0.0
---

# Adding a MageLift provider

Read `docs/adding-a-provider.md` and ADRs 0002, 0004, 0007, 0008 before coding.

## Boundary

- Magento-facing code stays in `sdk`, `internal/platform`, `internal/deploy`,
  `internal/config`
- VPC, DB, runtime, and Pulumi components stay under `internal/cloud/<provider>/`
- SaaS adapters go under `internal/external/<vendor>/`; provider-neutral ports go under `internal/shared/<port>/`; `internal/cloud/kube` is the single ADR-blessed shared K8s helper; SES and Cloudflare are adapter-less by decision
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
   until ready ([ADR 0004](../../../docs/adr/0004-ports-and-adapters.md))
5. `internal/config`: provider block + `make generate` for schema
6. First-party: register the `StackModule` in `internal/registry`. Community:
   implement `sdk.Module` and call `cli.NewWithExtensions(...)` from a custom
   binary.
7. Mock Pulumi graph tests; docs for experimental vs acceptance
8. Capability matrix row; never claim certified without acceptance evidence

## Honesty

Experimental providers are fine. Selling them as production-supported is not.

## Use this skill when

- You are adding a cloud provider, a runtime, or a managed service adapter.
- You are changing the intersection between Adobe compatibility and provider products.
- You are reviewing a community extension boundary.

## Leave behind

- Provider-specific Pulumi code under internal/cloud/<provider>/; SaaS code under internal/external/<vendor>/; shared ports under internal/shared/<port>/.
- Mock coverage and a capability-matrix row.
- A real acceptance row before changing a cell to certified.
