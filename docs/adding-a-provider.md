# Adding a provider

New clouds are adapters. Do not share Pulumi `Network`/`Database` components with
a provider switch (ADR 0002, 0004, 0008). Magento-facing code stays in `sdk/v1`,
`internal/platform`, `internal/deploy`, and `internal/config`. VPC, DB, and
runtime stay under `internal/cloud/<provider>/`.

## Registration (read first)

| Seam | Role |
| --- | --- |
| `platform.ModuleRegistry` | What the CLI uses. `preview` / `deploy` / `destroy` / `outputs` pick a `StackModule` by `target.provider` + `target.runtime`. |
| `internal/infra.Registry` | Target / Capability / Transform / Hook index for tests and SDK discovery. Registering here alone does not wire the CLI. |

A PR that only calls `infra.RegisterTarget` will not appear in `magelift deploy`.

## Checklist (copy GCP)

1. `internal/cloud/<p>/target/` — IDs, `Validate`, optional `Register(*infra.Registry)` for tests.
2. `internal/cloud/<p>/stack/` — `Spec`, `PlanFromConfig*`, Pulumi `Program` (`ctx.Export` must match `Component.Outputs()`), `component`, and a `StackModule`. If Magento Ops would cycle imports with other packages, put `HasOps` on a thin wrapper (AWS: `internal/cloud/aws/ops`).
3. Capability packages (`network`, `database`, …) as needed; keep them typed and small.
4. Optional day-2 ports on the module (`HasOps`, `HasBootstrap`, `HasState`,
   `HasSecrets`, `HasRuntimeObserve`). Return `ErrNotSupported` until ready.
   See [ADR 0009](adr/0009-day2-magento-ports.md). GCP’s certified module
   implements these in `internal/cloud/gcp/ops`.
5. `internal/config` — provider block, enums, validation, then `make generate` for schema.
6. `cmd/magelift` — `RegisterModule(...)` in the production binary (keep
   Pulumi SDKs out of `internal/cli` so `gendocs` and CLI unit tests stay
   linkable on small CI runners).
7. Mock Pulumi graph tests; docs for experimental vs acceptance.
8. Later for certification: implement the day-2 ports end-to-end (ADR 0007).

## Required stack outputs

`OutputKeys()` must include every key from `platform.RequiredOutputKeys()`. Map
native IDs into those names (e.g. GCP network → `networkVpcId`). Extra keys are
fine.

## Tiers

| Tier | Meaning |
| --- | --- |
| Certified | Shared Magento acceptance on a real account; full ops |
| Experimental | In-tree; may be infra-only; labeled in docs/CLI |
| Community | Out-of-tree module; custom binary calls `RegisterModule` |

Two certified first-party targets (AWS + GCP) already satisfy the multi-cloud
claim gate ([ADR 0007](adr/0007-multi-provider-community-targets.md)).

For AWS product choices (runtime, natMode, databaseEngine, searchMode), see the
matrix in [architecture.md](architecture.md#aws-magento-product-matrix). Full Magento
on AWS is not every SKU.

## Community binary

The released `magelift` binary includes first-party modules only. External
providers ship as a **compile-time custom binary** of this module (or a fork)
that calls `RegisterModule` — see `examples/custom-cli`. There is no Go
`plugin` ABI and no unsigned dynamic loader in v1 (ADR 0007).

**Honesty about `internal/`:** Go's visibility rule means a *separate* module
path cannot import `internal/platform` or `internal/cli`. Community providers
today live in this repository's module graph (custom `main` under
`examples/custom-cli`, or a fork). Do not claim publish-to-proxy.golang.org of
an external module that imports those packages. Exporting a public platform API
is a follow-on public API decision — not required to author an in-tree or
fork-based custom binary today.

### Clean-cache verification (RELEASE-03)

Use only this document and `examples/custom-cli` — do not browse
`internal/cloud/**` to learn registration. From the repo root (serial build;
never raise parallelism on a 16 GB host):

```sh
export GOMODCACHE="$(mktemp -d /tmp/magelift-modcache.XXXXXX)"
export GOCACHE="$(mktemp -d /tmp/magelift-gocache.XXXXXX)"
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli
/tmp/magelift-ext version
```

A green `version` line proves the custom binary builds from an empty module and
build cache without consulting core source beyond the example tree's imports.

## Non-goals

- Shared Pulumi components that switch on provider
- One YAML catalog for every cloud product
- Paid multi-cloud CI matrices by default
