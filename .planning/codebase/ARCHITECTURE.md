<!-- refreshed: 2026-07-27 -->
# Architecture

**Analysis Date:** 2026-07-27

## System Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                        CLI (Cobra)                           │
│  `internal/cli/` — build, dev, deploy(lifecycle), env,       │
│  secrets, state, releases, doctor, health, ci, upgrade        │
└───────────────┬──────────────────────┬──────────────────────┘
                │                       │
                ▼                       ▼
┌───────────────────────────┐  ┌───────────────────────────────┐
│   Config / SDK contracts   │  │      Platform (ports)         │
│ `internal/config/`         │  │ `internal/platform/`          │
│ `sdk/v1/` (topology,        │  │ StackModule, Ops, Bootstrap,  │
│  validation, types)         │  │ State, Secrets, CostEstimator │
└───────────────┬────────────┘  └───────────────┬───────────────┘
                │                                │
                ▼                                ▼
┌─────────────────────────────────────────────────────────────┐
│                Cloud adapters (per provider)                  │
│  `internal/cloud/aws/*` `internal/cloud/gcp/*`                │
│  `internal/cloud/ovh/*` `internal/cloud/scaleway/*`            │
│  each exposes: stack (Pulumi component + Plan), ops (day-2),   │
│  runtime, database, network, secrets, cache, naming, target    │
└───────────────┬─────────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────────────────────┐
│         Deploy orchestration & build pipeline                 │
│ `internal/deploy/` (Lock/Steps orchestrator)                  │
│ `internal/build/{pipeline,plan,runner,kit}` (image build)      │
│ `internal/automation/` (Pulumi automation API wrapper)         │
└─────────────────────────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────────────────────┐
│  External systems: Pulumi engine/state, cloud provider APIs   │
│  (AWS/GCP/OVH/Scaleway), container registries, Magento app     │
└─────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| CLI commands | Parse flags, load config, wire dependencies, print output | `internal/cli/root.go`, `internal/cli/lifecycle.go`, `internal/cli/build.go` |
| Config model | YAML/JSON schema for `magelift.yaml`, validation, presets | `internal/config/model.go`, `internal/config/presets.go` |
| SDK contracts | Cross-cutting types (`TargetDescriptor`, `Application`, `BuildArtifact`), topology + validation rules shared by every adapter | `sdk/v1/types.go`, `sdk/v1/topology.go`, `sdk/v1/validation.go` |
| Platform ports | Interfaces every cloud adapter must implement (`StackModule`, `Ops`, `Bootstrap`, `State`, `Secrets`, `CostEstimator`, `RuntimeObserve`) plus the `ModuleRegistry` that dispatches by provider/runtime | `internal/platform/module.go`, `internal/platform/ops.go`, `internal/platform/cost.go` |
| Cloud adapters | Provider-specific Pulumi component resources, plan/program construction, and day-2 ops (deploy steps, locks, releases) | `internal/cloud/aws/stack/component.go`, `internal/cloud/aws/ops/day2.go`, `internal/cloud/gcp/ops/day2.go`, `internal/cloud/ovh/stack/ops.go`, `internal/cloud/scaleway/stack/ops.go` |
| Deploy orchestrator | Provider-agnostic candidate deploy flow (validate → preview → register candidate → migrate → cleanup → update → stabilize → health → record) | `internal/deploy/orchestrator.go`, `internal/deploy/hooks.go` |
| Build pipeline | Multi-stage container image build orchestration (kit assembly, plan, runner execution) | `internal/build/pipeline/pipeline.go`, `internal/build/plan/plan.go`, `internal/build/runner/protocol.go`, `internal/build/kit/buildkit.go` |
| Automation wrapper | Thin layer over Pulumi Automation API (stack up/preview/destroy, change summaries) | `internal/automation/` |
| Local dev | Docker Compose-based local environment orchestration | `internal/localdev/compose.go` |
| Topology | Cross-provider infra shape resolution (what components a runtime/provider combo needs) | `internal/topology/` |

## Pattern Overview

**Overall:** Hexagonal / ports-and-adapters architecture layered over a Pulumi-based infrastructure-as-code core, fronted by a Cobra CLI.

**Key Characteristics:**
- Cloud providers are pluggable modules registered at startup (`cmd/magelift/main.go`) implementing the `platform.StackModule` interface; the CLI and deploy orchestrator never import concrete cloud packages directly — only through the registry and typed adapters.
- Optional capabilities (day-2 Magento deploy, release recording) are expressed as narrow interface extensions (`platform.HasOps`, `platform.HasRecordRelease`) rather than a single fat interface, so infrastructure-only adapters can omit them.
- `sdk/v1` acts as the stable contract package shared by all adapters and the core — it defines provider-agnostic types (`TargetDescriptor`, `Application`, `BuildArtifact`) and validation rules referenced by ADRs (e.g. `docs/adr/0002-provider-runtime-extension-boundary`, `0004-provider-package-layout`).
- Each cloud provider directory (`internal/cloud/{aws,gcp,ovh,scaleway}`) mirrors the same sub-package shape (`stack`, `ops`/`day2`, `runtime`, `database`, `network`, `secrets`, `cache`, `naming`, `target`), making it straightforward to add a new provider by cloning the shape.
- Certification tiers (`platform.TierCertified` / `TierExperimental`) gate which providers are production-ready vs experimental (AWS is certified; GCP/OVH/Scaleway are experimental per `docs/gcp-experimental.md`, `docs/ovh-experimental.md`, `docs/scaleway-experimental.md`).

## Layers

**CLI Layer:**
- Purpose: User-facing commands; wires config, modules, and infra backends together per invocation
- Location: `internal/cli/`
- Contains: Cobra command definitions (`root.go`, `lifecycle.go`, `build.go`, `dev.go`, `env.go`, `secrets.go`, `state.go`, `releases.go`, `ci.go`, `doctor.go`, `health.go`, `upgrade.go`, `cost.go`, `ports.go`)
- Depends on: `internal/config`, `internal/platform`, `internal/deploy`, `internal/automation`, `internal/cosign`, `internal/releasejournal`, `internal/upgrade`
- Used by: `cmd/magelift/main.go` entry point

**Config/SDK Layer:**
- Purpose: Define and validate the `magelift.yaml` schema and shared cross-provider domain types
- Location: `internal/config/`, `sdk/v1/`
- Contains: Struct definitions with `yaml`/`json`/`config`/`schema` tags used to generate `schema/magelift.schema.json` (via `cmd/genconfig`), validation logic, topology resolution
- Depends on: nothing internal (leaf package)
- Used by: every other layer

**Platform (Ports) Layer:**
- Purpose: Define the contracts cloud adapters must satisfy; provide the module registry that dispatches by `(provider, runtime)`
- Location: `internal/platform/`
- Contains: `StackModule`, `Ops`, `Bootstrap`, `State`, `Secrets`, `CostEstimator`, `RuntimeObserve` interfaces; `ModuleRegistry`; output-key constants
- Depends on: `internal/config`, `sdk/v1`, `internal/deploy` (for the `Steps` type used in `Ops.NewDeploySteps`), Pulumi SDK
- Used by: CLI layer, all cloud adapters (implement its interfaces)

**Cloud Adapter Layer:**
- Purpose: Translate provider-agnostic plans into Pulumi resource graphs and provider-specific day-2 operations
- Location: `internal/cloud/aws/`, `internal/cloud/gcp/`, `internal/cloud/ovh/`, `internal/cloud/scaleway/`, `internal/cloud/kube/`
- Contains: `stack/component.go` (Pulumi `ComponentResource`), `stack/spec.go` (resolved spec), `stack/module.go` (StackModule impl), `ops/day2.go` or `eksops/ops.go` (Ops impl), plus per-concern packages (`network`, `database`, `secrets`, `cache`, `runtime`, `naming`, `target`, `queue`, `edge`, `ingress`, `observability`, `cost`, `pricing`, `state`, `storage`, `search`)
- Depends on: `internal/platform`, `internal/config`, `sdk/v1`, provider Pulumi SDKs (`pulumi-aws`, `pulumi-gcp`, `pulumi-ovh`, `pulumi-scaleway`, `pulumi-kubernetes`)
- Used by: `cmd/magelift/main.go` (registration), `internal/platform.ModuleRegistry` (dispatch)

**Deploy Orchestration Layer:**
- Purpose: Provider-agnostic candidate deploy state machine (Magento-specific: build → migrate → cutover → health-check)
- Location: `internal/deploy/`
- Contains: `orchestrator.go` (`Steps` interface, `Request`/`Result` types), `hooks.go`, `registry.go`, `ownership.go`
- Depends on: `internal/automation`, `sdk/v1`
- Used by: `internal/cli/lifecycle.go`, each provider's `ops`/`day2` package (implements `deploy.Steps`)

**Build Layer:**
- Purpose: Build Magento container images through a pluggable, protocol-driven pipeline
- Location: `internal/build/`
- Contains: `pipeline/pipeline.go` (stage orchestration), `plan/plan.go` (build plan resolution), `runner/protocol.go` + `runner/client.go` + `runner/codec.go` (subprocess/runner protocol), `kit/buildkit.go` (BuildKit invocation)
- Depends on: `internal/config`, `sdk/v1`
- Used by: `internal/cli/build.go`, `internal/cli/ci.go`

**Automation Layer:**
- Purpose: Wraps Pulumi's Automation API (inline programs, stack lifecycle, change summaries)
- Location: `internal/automation/`
- Depends on: `github.com/pulumi/pulumi/sdk/v3`
- Used by: `internal/cli/root.go` (infrastructure backend), cloud adapter `ops` packages

**Support Layers:**
- `internal/localdev/` — Docker Compose local environment (`compose.go`)
- `internal/topology/` — cross-provider infra shape resolution
- `internal/health/`, `internal/releasejournal/`, `internal/cosign/`, `internal/secretref/`, `internal/upgrade/`, `internal/toolchain/`, `internal/containerrunner/`, `internal/source/`, `internal/infra/`, `internal/usererr/` — focused single-purpose leaf packages used by the CLI

## Data Flow

### Primary Deploy Path

1. User runs `magelift deploy` / lifecycle command → Cobra command handler in `internal/cli/lifecycle.go`
2. CLI loads and validates `magelift.yaml` via `internal/config` and resolves the target module via `platform.ModuleRegistry.Plan()` (`internal/platform/module.go:120`)
3. Selected `StackModule.Plan()` (e.g. `internal/cloud/aws/stack/module.go`) builds a `PlannedStack` without touching the cloud
4. CLI opens an infrastructure backend (Pulumi Automation stack) via `newBackend` in `internal/cli/root.go`, then calls `StackModule.Program()` to register Pulumi resources (`internal/cloud/aws/stack/component.go`)
5. For Magento application deploys, CLI resolves `platform.ModuleOps(module)` and drives `deploy.Steps` (Validate → Preview → RegisterCandidate → RunMigrations → CleanupCandidate → UpdateServices → Stabilize → Health → Record) defined in `internal/deploy/orchestrator.go`
6. Provider `ops`/`day2` package implements each `Steps` method against the concrete infra (ECS, EKS, Cloud Run, etc.)
7. Release outcome recorded via `internal/releasejournal/`

### Build Path

1. `magelift build` (`internal/cli/build.go`) resolves a build plan (`internal/build/plan/plan.go`) from `config.Build`
2. Pipeline stages executed (`internal/build/pipeline/pipeline.go`), invoking BuildKit (`internal/build/kit/buildkit.go`) and/or the runner subprocess protocol (`internal/build/runner/protocol.go`, `client.go`, `codec.go`)
3. Resulting image digest flows into `sdk.BuildArtifact`, consumed by deploy orchestration

**State Management:**
- Infrastructure state lives in Pulumi backends (per-provider `state` packages, e.g. `internal/cloud/aws/state/`), not in-process.
- CLI-level runtime state is passed explicitly through function parameters and small option structs (`options` in `internal/cli/root.go`) — no global mutable state observed.

## Key Abstractions

**PlannedStack (interface):**
- Purpose: Opaque, validated, immutable-ish representation of a resolved deployment target
- Examples: `internal/platform/module.go:29`, implemented per-provider (e.g. AWS spec in `internal/cloud/aws/stack/spec.go`)
- Pattern: Value object with a `WithImageDigest` "wither" method for post-plan digest injection

**StackModule (interface):**
- Purpose: The single extension point new cloud providers implement
- Examples: `internal/cloud/aws/stack/module.go`, `internal/cloud/gcp/.../module.go` (equivalent), `internal/cloud/ovh/stack/`, `internal/cloud/scaleway/stack/`
- Pattern: Strategy/plugin pattern; selected at runtime by `(Provider, Runtime)` key

**Ops / HasOps (interface):**
- Purpose: Optional capability interface for day-2 Magento operations, kept separate from infra provisioning
- Examples: `internal/cloud/aws/ops/day2.go`, `internal/cloud/aws/eksops/ops.go`, `internal/cloud/gcp/ops/day2.go`
- Pattern: Interface segregation / optional capability via type assertion (`platform.ModuleOps`)

**deploy.Steps (interface):**
- Purpose: Provider-agnostic template method for the Magento candidate-deploy sequence
- Examples: `internal/deploy/orchestrator.go:39-49`
- Pattern: Template method / strategy — CLI drives fixed step order, provider supplies implementation

**TargetDescriptor / sdk types:**
- Purpose: Stable, versioned contract types shared across the module boundary (ADR-governed)
- Examples: `sdk/v1/types.go`, `sdk/v1/topology.go`
- Pattern: Anti-corruption layer / shared kernel between core and adapters

## Entry Points

**CLI binary:**
- Location: `cmd/magelift/main.go`
- Triggers: `magelift` command invocation
- Responsibilities: Register all cloud `StackModule`s into a `platform.ModuleRegistry`, construct the Cobra root command via `cli.NewWithModules`, execute, map errors to exit codes

**Config schema generator:**
- Location: `cmd/genconfig/`
- Triggers: `make` target / CI, regenerates `schema/magelift.schema.json` from `internal/config` struct tags

**Docs generator:**
- Location: `cmd/gendocs/`
- Triggers: `make` target, generates CLI reference docs from Cobra command tree

## Architectural Constraints

- **Threading:** Primarily single-threaded CLI execution per command; Pulumi Automation API and provider SDKs may use internal goroutines/worker pools not exposed to callers.
- **Global state:** None observed at the package level in `internal/cli` or `internal/platform` — dependencies are injected through the `options` struct and constructor functions (`NewWithModules`), not package-level singletons.
- **Provider boundary:** By ADR 0002/0004, cloud adapter internals must not leak into the CLI or `internal/deploy`; only `sdk/v1` types and `platform` interfaces cross the boundary. Adapters type-assert `backend any` in `Ops.NewDeploySteps` to reach provider-specific backend methods (`internal/platform/ops.go:24`).
- **Certification tiers:** Experimental providers (GCP, OVH, Scaleway) may have thinner test coverage and no DIY lock (`AcquireLock` may return a no-op release) — see `internal/platform/ops.go:19-20`.

## Anti-Patterns

### `backend any` parameter in Ops.NewDeploySteps

**What happens:** `Ops.NewDeploySteps(ctx, backend any, ...)` accepts an untyped `any` for the infrastructure backend (`internal/platform/ops.go:24`).
**Why it's wrong:** Loses compile-time safety; each adapter must type-assert the concrete backend type, which can panic if the CLI passes the wrong implementation.
**Do this instead:** This is a deliberate boundary-crossing pattern documented in the interface comment ("adapters type-assert as needed") — treat it as an accepted, narrow exception rather than something to replicate elsewhere; prefer typed function signatures for any new cross-package calls.

## Error Handling

**Strategy:** Standard Go error wrapping (`fmt.Errorf("...: %w", err)`) with sentinel errors for well-known conditions.

**Patterns:**
- Sentinel errors declared at package scope (`deploy.ErrApprovalRequired`, `deploy.ErrDigestRequired`, `deploy.ErrForwardOnlyRollbackAck`, `deploy.ErrLockRelease` in `internal/deploy/orchestrator.go:16-19`; `platform.ErrNotSupported` in `internal/platform/ops.go:13`)
- CLI maps errors to process exit codes via a custom `exitError` type implementing `Unwrap` for `errors.As` compatibility (`internal/cli/root.go:29-42`)
- User-facing errors distinguished via `internal/usererr/` package

## Cross-Cutting Concerns

**Logging:** Diagnostics routed through explicit `io.Writer` parameters (stdout/stderr/diagnostics writer) rather than a global logger — see `deploy.Steps` methods accepting `diagnostics io.Writer` and `options.stdout`/`options.stderr` in `internal/cli/root.go`.
**Validation:** Centralized in `sdk/v1/validation.go` (topology/target validation) and `internal/config` (schema validation); enforced at `ModuleRegistry.RegisterModule` (output key completeness, certification tier) and `StackModule.Plan` (target/config validation).
**Authentication:** Delegated to provider SDK credential chains (AWS SDK config/credentials, GCP ADC, etc.); OIDC-based CI auth documented in `docs/adr/0006-github-oidc-role-separation.md`.

---

*Architecture analysis: 2026-07-27*
</content>
