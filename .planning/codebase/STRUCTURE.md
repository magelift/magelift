# Codebase Structure

**Analysis Date:** 2026-07-27

## Directory Layout

```
magelift/
├── cmd/                    # Go main packages (binaries)
│   ├── magelift/           # Primary CLI entry point
│   ├── genconfig/          # Generates schema/magelift.schema.json from internal/config
│   └── gendocs/            # Generates CLI reference docs
├── internal/                # Private application code (not importable externally)
│   ├── cli/                 # Cobra commands, wiring, CLI-facing option structs
│   ├── config/              # magelift.yaml schema, presets, validation
│   ├── platform/            # Ports: StackModule, Ops, Bootstrap, State, Secrets, CostEstimator
│   ├── cloud/                # Cloud provider adapters (one subtree per provider)
│   │   ├── aws/              # Certified provider: stack, ops, eksops, network, database, ...
│   │   ├── gcp/               # Experimental provider, mirrors aws/ shape
│   │   ├── ovh/                # Experimental provider, mirrors aws/ shape (fewer sub-packages)
│   │   ├── scaleway/            # Experimental provider, mirrors aws/ shape
│   │   └── kube/                 # Shared Kubernetes helpers (used by eks/gke-based runtimes)
│   ├── deploy/                # Provider-agnostic candidate deploy orchestrator (Steps/Lock)
│   ├── build/                 # Container image build pipeline
│   │   ├── pipeline/            # Stage orchestration
│   │   ├── plan/                 # Build plan resolution from config
│   │   ├── runner/                # Subprocess build-runner protocol (client/codec)
│   │   └── kit/                    # BuildKit invocation
│   ├── automation/            # Pulumi Automation API wrapper
│   ├── topology/              # Cross-provider infra shape resolution
│   ├── localdev/              # Docker Compose local dev environment
│   ├── health/                # Health check logic
│   ├── releasejournal/        # Release history recording
│   ├── cosign/                 # Image signature verification
│   ├── secretref/              # Secret reference resolution helpers
│   ├── upgrade/                 # Self-upgrade logic for the CLI binary
│   ├── toolchain/                # External tool (pulumi, docker, etc.) discovery/versioning
│   ├── containerrunner/          # Container execution abstraction
│   ├── source/                    # Source repo / branch helpers
│   ├── infra/                      # Shared infra helpers
│   ├── usererr/                     # User-facing error classification
│   └── benchmark/                    # Benchmark command support
├── sdk/v1/                  # Stable, ADR-governed contract types shared by core + adapters
├── schema/                  # Generated `magelift.schema.json` (do not hand-edit)
├── docs/                    # ADRs, guides, generated CLI reference (mkdocs source)
│   ├── adr/                  # Architecture decision records
│   ├── sources/               # Reference source material
│   └── knowledge/              # OKF knowledge bundle (index.md, lessons/, reference/)
├── site/                    # Built mkdocs site output (generated, not hand-edited)
├── images/                  # Dockerfiles for runtime images (frankenphp, php-runtime, varnish)
├── examples/                # Example projects (custom-cli, sample-shop)
├── scripts/                 # Bash scripts for acceptance/smoke testing, local acceptance runs
├── build/                   # PHP-based Magento build support (Artifact/Lifecycle/Magento/Process/Protocol/Runner + tests) — separate PHP subsystem consumed by internal/build/runner
├── .github/workflows/       # CI pipelines
└── .planning/               # GSD planning artifacts (this document lives here)
```

## Directory Purposes

**`cmd/magelift/`:**
- Purpose: Wires all cloud `StackModule`s into a registry and starts the Cobra CLI
- Contains: Single `main.go`
- Key files: `cmd/magelift/main.go`

**`internal/cli/`:**
- Purpose: All user-facing commands and their option/dependency wiring
- Contains: One file per command area plus a matching `_test.go`, and `root.go` for shared plumbing (options struct, exit codes, backend/lock/secret factories)
- Key files: `internal/cli/root.go` (shared wiring), `internal/cli/lifecycle.go` (deploy flow), `internal/cli/build.go`, `internal/cli/ci.go`, `internal/cli/dev.go`, `internal/cli/env.go`, `internal/cli/secrets.go`, `internal/cli/state.go`

**`internal/config/`:**
- Purpose: Defines the `magelift.yaml` schema (Go structs with `yaml`/`json`/`config`/`schema` tags), presets, and validation
- Contains: `model.go` (root `Config` struct + nested types), `presets.go`, `repository.go` (load/parse)
- Key files: `internal/config/model.go`, `internal/config/presets.go`

**`internal/platform/`:**
- Purpose: The port layer — interfaces every cloud adapter implements, plus the module registry used for runtime dispatch
- Contains: `module.go` (`StackModule`, `PlannedStack`, `ModuleRegistry`), `ops.go` (`Ops`, `HasOps`, `HasRecordRelease`), `cost.go` (`CostEstimator`), `account.go`, `env.go`, `observe.go`, `outputs.go`, `stackname.go`, `workloads.go`, `compose.go`
- Key files: `internal/platform/module.go`, `internal/platform/ops.go`

**`internal/cloud/<provider>/`:**
- Purpose: Provider-specific Pulumi resource graph construction and day-2 operations
- Contains (per provider, AWS has the fullest set): `stack/` (component, spec, config, module, program, providers), `ops/` or `eksops/` (day-2 deploy steps), and focused sub-packages: `network`, `database`, `secrets`, `cache`, `runtime`, `naming`, `target`, `queue`, `edge`, `ingress`, `observability`, `cost`, `pricing`, `state`, `storage`, `search`, `bootstrap`, `deployment`, `endpoint`, `eks`, `operations`
- Key files: `internal/cloud/aws/stack/component.go` (Pulumi ComponentResource, largest file at ~456 lines), `internal/cloud/aws/stack/spec.go`, `internal/cloud/aws/ops/day2.go`, `internal/cloud/aws/eksops/ops.go`, `internal/cloud/gcp/ops/day2.go`, `internal/cloud/ovh/stack/ops.go`, `internal/cloud/scaleway/stack/ops.go`

**`internal/deploy/`:**
- Purpose: Provider-agnostic Magento candidate-deploy state machine (the `Steps` interface implemented by each provider's `ops` package)
- Contains: `orchestrator.go`, `hooks.go`, `registry.go`, `ownership.go`
- Key files: `internal/deploy/orchestrator.go`

**`internal/build/`:**
- Purpose: Builds Magento application container images
- Contains: `pipeline/` (stage orchestration + adapters), `plan/` (resolves what to build from config), `runner/` (protocol/client/codec for a subprocess build runner), `kit/` (BuildKit driver)
- Key files: `internal/build/pipeline/pipeline.go`, `internal/build/plan/plan.go`, `internal/build/runner/protocol.go`

**`sdk/v1/`:**
- Purpose: Stable, versioned contract types crossing the core/adapter boundary; governed by ADRs (e.g. `docs/adr/0002-provider-runtime-extension-boundary`)
- Contains: `types.go` (domain types), `topology.go` (infra shape resolution), `validation.go`
- Key files: `sdk/v1/types.go`, `sdk/v1/validation.go`

**`schema/`:**
- Purpose: Generated JSON Schema for `magelift.yaml`, produced by `cmd/genconfig` from `internal/config` struct tags
- Generated: Yes — do not hand-edit `schema/magelift.schema.json`

**`docs/`:**
- Purpose: ADRs, provider guides (AWS/GCP/OVH/Scaleway acceptance and experimental docs), operations/configuration reference, mkdocs source
- Contains: `adr/` (numbered ADRs, e.g. `docs/adr/0010-database-dump-seed.md`), `knowledge/` (OKF bundle: `index.md`, `lessons/`, `reference/`), `sources/`
- Key files: `docs/architecture.md`, `docs/configuration.md`, `docs/capability-matrix.md`

**`site/`:**
- Purpose: Built mkdocs static site output
- Generated: Yes; Committed: appears tracked (contains built HTML) — treat as build artifact, avoid manual edits

**`build/` (PHP subsystem):**
- Purpose: A separate PHP codebase (`build/src/{Artifact,Lifecycle,Magento,Process,Protocol,Runner}` with mirrored `build/tests/`) that implements the Magento-side build/runner protocol consumed by `internal/build/runner`
- Contains: PHP source and PHPUnit-style tests
- Note: This is a distinct language/toolchain from the Go core — treat as its own module when making changes (check for its own `composer.json`/build tooling before editing)

**`images/`:**
- Purpose: Dockerfiles and configs for runtime container images (`frankenphp-classic/`, `php-runtime/`, `varnish/`)

**`examples/`:**
- Purpose: Example projects demonstrating custom CLI extension (`custom-cli/`) and a full sample Magento shop config (`sample-shop/`)

**`scripts/`:**
- Purpose: Bash scripts for acceptance/smoke testing against real cloud accounts and local Docker acceptance runs (not unit tests)
- Key files: `scripts/aws-acceptance-local.sh`, `scripts/gcp-acceptance-local.sh`, `scripts/release-smoke-local.sh`, `scripts/time-to-preview.sh`

## Key File Locations

**Entry Points:**
- `cmd/magelift/main.go`: CLI binary, registers all `StackModule`s
- `cmd/genconfig/`: Regenerates `schema/magelift.schema.json`
- `cmd/gendocs/`: Regenerates CLI reference docs

**Configuration:**
- `internal/config/model.go`: `magelift.yaml` schema definition
- `schema/magelift.schema.json`: Generated JSON Schema (consumed by editors/CI validation)
- `.golangci.yml`: Lint configuration
- `.goreleaser.yaml`: Release packaging configuration

**Core Logic:**
- `internal/platform/module.go`: Module registry / port definitions
- `internal/deploy/orchestrator.go`: Deploy state machine
- `internal/cloud/aws/stack/component.go`: Reference implementation of a full Pulumi component (largest, most mature adapter)

**Testing:**
- Go tests are co-located: `internal/cli/lifecycle_test.go` next to `internal/cli/lifecycle.go`, same pattern across `internal/`
- `scripts/*.sh`: acceptance/smoke test scripts, run outside `go test`
- `build/tests/`: PHP-side test suite mirroring `build/src/`

## Naming Conventions

**Files:**
- Go source files use lowercase snake-free names matching the primary type/concern (`component.go`, `spec.go`, `module.go`, `orchestrator.go`); tests are `<name>_test.go` co-located in the same package
- Provider "day-2 ops" files are named `day2.go` (AWS/GCP) or `ops.go` (OVH/Scaleway/EKS variant), implementing `platform.Ops`

**Directories:**
- Cloud provider sub-packages follow a consistent vocabulary across providers: `stack`, `ops`/`day2`/`eksops`, `network`, `database`, `secrets`, `cache`, `runtime`, `naming`, `target` — new providers should adopt the same sub-package names for consistency, even if AWS (the most mature) has more of them (`queue`, `edge`, `ingress`, `observability`, `cost`, `pricing`, `search`, `state`, `storage`, `bootstrap`, `deployment`, `endpoint`, `eks`, `operations`)
- ADRs in `docs/adr/` are numbered sequentially (`000N-title.md`)

## Where to Add New Code

**New Cloud Provider:**
- Create `internal/cloud/<provider>/` mirroring the OVH or Scaleway shape (`stack/`, `ops.go`, `network/`, `database/`, `secrets/`, `cache/`, `runtime/`, `naming/`, `target/`)
- Implement `platform.StackModule` in `stack/module.go`; implement `platform.Ops` in `ops.go` if day-2 Magento deploy is supported
- Register the module in `cmd/magelift/main.go`
- Read `docs/adr/0002-provider-runtime-extension-boundary.md` and `docs/adr/0004-provider-package-layout.md` first — they define the required boundary and layout
- Add provider docs following the pattern of `docs/ovh-experimental.md` / `docs/scaleway-experimental.md`

**New CLI Command:**
- Add `internal/cli/<command>.go` + `internal/cli/<command>_test.go`; wire into `internal/cli/root.go`
- Follow the existing pattern of accepting dependencies through the `options` struct rather than reaching for globals

**New Deploy Step / Behavior Change:**
- Modify `internal/deploy/orchestrator.go` (the `Steps` interface) — this is a breaking change for every provider's `ops`/`day2` implementation, so update all of `internal/cloud/{aws,gcp,ovh,scaleway}/.../day2.go|ops.go` together

**New Config Field:**
- Add the field to the relevant struct in `internal/config/model.go` with `yaml`/`json`/`config`/`schema` tags
- Regenerate `schema/magelift.schema.json` via `cmd/genconfig` (do not hand-edit the generated file)

**Shared Cross-Provider Type:**
- Add to `sdk/v1/types.go` only if it must cross the platform/adapter boundary; otherwise keep it local to the adapter package (per the provider-runtime-extension-boundary ADR)

**Utilities:**
- Provider-agnostic helpers: `internal/topology/`, `internal/toolchain/`, `internal/source/`, `internal/infra/`
- Avoid adding a generic `internal/utils` package — the codebase favors focused, single-purpose leaf packages named after their concern (`usererr`, `secretref`, `cosign`)

## Special Directories

**`schema/`:**
- Purpose: Generated JSON Schema for the config file
- Generated: Yes (via `cmd/genconfig`)
- Committed: Yes

**`site/`:**
- Purpose: Built mkdocs static site
- Generated: Yes
- Committed: Yes (tracked in this repo snapshot)

**`build/` (PHP):**
- Purpose: Separate PHP subsystem for the Magento-side build/runner protocol
- Generated: No (hand-written PHP source + tests)
- Committed: Yes — note it uses a different toolchain than the Go core

**`dist/`:**
- Purpose: GoReleaser build output (e.g. `dist/magelift_darwin_arm64_v8.0`)
- Generated: Yes
- Committed: Should not be committed (build artifact) — verify `.gitignore` coverage before adding files here

**`.magelift/`:**
- Purpose: Local runtime/working state for magelift itself (e.g. `gcp-matrix/logs`)
- Generated: Yes
- Committed: Working/log data — treat as ephemeral

---

*Structure analysis: 2026-07-27*
</content>
