---
status: done
slug: sever-core-imports
intent: intent.md
---

# Spec: sever core imports

## Requirements

### Requirement: core CLI entrypoint imports zero provider packages

Non-test Go code under `cli/` SHALL import zero `internal/cloud/*` packages.

#### Scenario: direct cloud imports are gone from cli

- **WHEN** the change is complete
- **THEN** `grep -rn '"github.com/magelift/magelift/internal/cloud' cli/ --include='*.go' | grep -v '_test.go'` returns empty.

#### Scenario: cli still builds without provider hook constructors

- **WHEN** the hooks aggregator owns construction
- **THEN** `grep -rn 'awssecrets\|gcpsecrets\|gcpresilience\|gcpstack\|gcp/naming' cli/ --include='*.go' | grep -v '_test.go'` returns empty.

### Requirement: internal CLI imports zero provider packages

Non-test Go code under `internal/cli/` SHALL import zero `internal/cloud/*` packages.

#### Scenario: direct cloud imports are gone from internal/cli

- **WHEN** the change is complete
- **THEN** `grep -rn '"github.com/magelift/magelift/internal/cloud' internal/cli/ --include='*.go' | grep -v '_test.go'` returns empty.

#### Scenario: go list confirms no direct cloud dependency

- **WHEN** inspected via the Go tool
- **THEN** `go list -f '{{ join .Imports "\n" }}' ./internal/cli/ | grep 'internal/cloud'` returns empty.

### Requirement: released binary resolves hooks through explicit registration

The released binary SHALL resolve all provider hooks through an explicit `RegisterHooks` call from `cmd/magelift`, with no provider `init()` registration and no hidden wiring inside `cli/` or `internal/cli/`.

#### Scenario: cmd/magelift is the explicit registration site

- **WHEN** the released entrypoint is inspected
- **THEN** `grep -rn 'RegisterHooks' cmd/magelift/ --include='*.go'` returns non-empty.

#### Scenario: no init-magic registration

- **WHEN** the core and registry are inspected
- **THEN** `grep -rn 'func init(' cli/ internal/cli/ internal/registry/ --include='*.go' | grep -v '_test.go'` returns empty.

### Requirement: CLI behavior is preserved

CLI behavior SHALL be unchanged: existing suites stay green, the custom-binary example still builds, and the offline gate stays green.

#### Scenario: internal/cli suite green

- **WHEN** the CLI suite runs
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1` exits `0`.

#### Scenario: registry suite green

- **WHEN** the registry suite runs
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/registry/ -count=1` exits `0`.

#### Scenario: custom binary still builds

- **WHEN** the community example builds
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./examples/custom-cli/` exits `0`.

#### Scenario: offline gate green

- **WHEN** the repo gate runs
- **THEN** `make verify` exits `0`.

### Requirement: gendocs stays linkable without Pulumi in internal/cli

Non-test Go code under `internal/cli/` SHALL NOT import Pulumi SDKs, so `cmd/gendocs` and CLI unit tests stay linkable on small runners.

#### Scenario: no Pulumi SDK imports in non-test internal/cli

- **WHEN** inspected
- **THEN** `grep -rn 'pulumi/pulumi/sdk' internal/cli/ --include='*.go' | grep -v '_test.go'` returns empty.

#### Scenario: gendocs builds

- **WHEN** the docs generator builds
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./cmd/gendocs/` exits `0`.

### Requirement: registry still wires all six first-party modules

`internal/registry.NewDefault` SHALL register all six first-party `StackModule`s (aws ECS, aws EKS, gcp Autopilot, gcp Standard, ovh, scaleway) in-process.

#### Scenario: six module entries present

- **WHEN** the registry source is inspected
- **THEN** `grep -c 'Module{' internal/registry/registry.go` returns `6`.

#### Scenario: NewDefault entrypoint retained

- **WHEN** the registry API is inspected
- **THEN** `grep -rn 'func NewDefault' internal/registry/ --include='*.go' | grep -v '_test.go'` returns non-empty.

### Requirement: SDK and config stay provider-free

Non-test Go code under `sdk/v1/` and `internal/config/` SHALL import zero `internal/cloud/*` packages (regression lock on the verified-clean state).

#### Scenario: sdk/v1 has no cloud imports

- **WHEN** inspected
- **THEN** `grep -rn '"github.com/magelift/magelift/internal/cloud' sdk/v1/ --include='*.go' | grep -v '_test.go'` returns empty.

#### Scenario: internal/config has no cloud imports

- **WHEN** inspected
- **THEN** `grep -rn '"github.com/magelift/magelift/internal/cloud' internal/config/ --include='*.go' | grep -v '_test.go'` returns empty.

## Design

Current state (researched, read-only). `cli/cli.go` holds 5 direct cloud imports (`aws/secrets`, `gcp/naming`, `gcp/resilience`, `gcp/secrets`, `gcp/stack`) behind `firstPartyHooks()`, which populates the 4-field `internal/cli.Hooks` struct (`NewComposerSecrets`, `NewComposerGCPSecrets`, `NewLeftoverBackupDestroyer`, `NewGCPCleanupProvider`) plus two unexported glue types (`gcpComposerSecretAdapter`, `gcpCloudSQLLeftoverBackupDestroyer`). `internal/cli/cleanup.go` holds 3 direct cloud imports (`gcp/resilience`, `ovh/resilience`, `scaleway/resilience`) inside `defaultCleanupProvider`, which switches on `ledger.Provider`. `internal/cli/env.go` holds 1 direct cloud import (`aws/endpoint.FromEnv`) inside `newMediaS3Client`. `internal/cli/sign.go` mentions `internal/cloud/gcp/bootstrap` in a comment only, not an import. `internal/cli/execute.go` is clean. `sdk/v1` and `internal/config` are clean. `internal/registry.NewDefault` registers the 6 first-party modules. `cmd/magelift/main.go` today calls `cli.New()` and does no explicit registration.

Proposed shape (packages, not files; the plan lists files):

```go
// internal/registry owns the first-party aggregation (may import internal/cloud/*).
func RegisterHooks() internalcli.Hooks

// internal/cli keeps owning the port shapes (imports platform + sdk/v1 only).
type Hooks struct {
    NewComposerSecrets         func(context.Context, string) (ComposerSecretProvider, error)
    NewComposerGCPSecrets      func(context.Context) (ComposerSecretProvider, error)
    NewLeftoverBackupDestroyer func(context.Context, platform.PlannedStack) (LeftoverBackupDestroyer, error)
    NewGCPCleanupProvider      func(context.Context, sdk.CleanupLedger) (CleanupProvider, error)
    NewCleanupProvider         func(context.Context, sdk.CleanupLedger) (CleanupProvider, error)
    MediaEndpoint              func() (string, error)
}
```

How it fits:

- `internal/cli` keeps owning the port interfaces (`ComposerSecretProvider`, `LeftoverBackupDestroyer`, `CleanupProvider`) and the dispatch/fallback logic. `defaultCleanupProvider` shrinks to hook dispatch: prefer `newCleanupProvider`, then `newGCPCleanupProvider` for `gcp` ledgers, else `cleanup.UnsupportedProvider`. `newMediaS3Client` reads the endpoint from the injected `MediaEndpoint` hook instead of importing `aws/endpoint`. All existing `options` test seams stay.
- `internal/registry` gains the `RegisterHooks()` aggregator. It is the single core-adjacent place allowed to import the 9 provider hook packages for construction. It moves today's `firstPartyHooks()` body, the GCP composer adapter (including the `GetParameter` refusal), the leftover-destroyer glue, and the `gcp`/`ovh`/`scaleway` cleanup constructors out of `cli/` and `internal/cli/`. Provider packages export unchanged constructors: `aws/secrets.New`, `gcp/secrets.NewStore`, `gcp/stack.AsGCPPlanned`, `gcp/naming.CloudSQLInstance`, `gcp/resilience.NewGCPCloudSQLNativeAPI` and `NewCloudSQLAPI`, `ovh/resilience.NewOVHClientFromProfile` and `NewOVHDatabaseNativeAPI`, `scaleway/resilience.NewScalewayDatabaseNativeAPI`, `aws/endpoint.FromEnv`.
- `cmd/magelift/main.go` becomes the explicit registration site: `modules, _ := registry.NewDefault()`, `hooks := registry.RegisterHooks()`, then construct the command through a hooks-accepting constructor. No `init()`, no globals, no hidden wiring.
- `cli.NewWithExtensions` keeps its exact signature `func NewWithExtensions(extensions ...sdk.Module) (*cobra.Command, error)` for custom-binary compatibility (`examples/custom-cli`, docs, ROADMAP promise). Internally it drops all cloud imports and delegates hook resolution to `registry.RegisterHooks()`; a new sibling constructor accepting an explicit `internalcli.Hooks` value serves `cmd/magelift` and custom binaries that need different hooks. The plan picks the sibling's name.
- `internal/cleanup` provider structs (`GCPCloudSQLProvider`, `OVHDatabaseProvider`, `ScalewayDatabaseProvider`) are unchanged; they already accept injected native clients, so only client construction moves.
- `internal/platform` (`ModuleRegistry`, `StackModule`) and `infra.RegisterTarget` semantics are unchanged per ADR 0004 and `docs/adding-a-provider.md`: modules still wire through `cmd/magelift` / `platform.ModuleRegistry`; `infra.RegisterTarget` alone still does not ship `magelift deploy`.

## Gotchas / policy flags

- Secret handling stays reference-only. Composer credentials resolve at build/run time from `secretref.Parse` references; no secret values in YAML, logs, or evidence. The GCP adapter's `GetParameter` refusal message and the AWS region-from-ARN logic in `secretRegion` (core, no cloud import) are preserved verbatim in behavior.
- No behavior change to secret resolution, cleanup validation errors (`GCP cleanup ledgers require a project`, OVH/Scaleway profile/region/project requirements), or the AWS endpoint loopback refusal. Error strings move packages only if the plan keeps them identical; prefer moving constructors without rewording.
- v1 single-version promise kept: one `go.mod`, in-process modules, no per-provider releases, no `go.work`, no build tags.
- In-process modules unchanged: the GCP subprocess proof cell, `providerhost` Dial, and day-2 ports staying in-process are untouched.
- Custom-binary compatibility: `cli.NewWithExtensions` signature stable; `sdk.Module` remains the only public extension boundary (no `internal/` imports for extensions); `examples/custom-cli` must build.
- Verification patterns must match quoted import paths (`"github.com/magelift/magelift/internal/cloud`), not bare `internal/cloud`, because `internal/cli/sign.go` contains a comment-only mention of a provider path.
- No hand-edits of generated files; `make generate` if schema, CLI reference, coverage, or `agents/manifest.json` are touched (not expected for this change).
- Narrow test loops per repo rules: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/<pkg>/ -count=1`; no unbounded `-race ./...` or parallel heavy builds from IDE sessions.

## Open questions carried forward

- Hook registration shape — DECIDED (validated decision, recorded): explicit `RegisterHooks` called from `cmd/magelift`. Rejected: provider `init()` magic and pure ports-only with no registration call. Default applied; no owner needed. The plan implements `registry.RegisterHooks()` plus the hooks-accepting `cli` constructor.
- Port shapes covering cleanup (`LeftoverBackupDestroyer`, `CleanupProvider`) and env (`aws/endpoint`) without leaking SDK types — PROPOSED default: one generic `NewCleanupProvider(ctx, sdk.CleanupLedger) (CleanupProvider, error)` for ledger dispatch (subsuming the ovh/scaleway/gcp-fallback switch; keep the existing `NewGCPCleanupProvider` field populated by the same aggregator for seam continuity) plus one `MediaEndpoint() (string, error)` hook for the S3 endpoint override. All provider SDK types stay behind constructors; only `string`, `sdk.CleanupLedger`, and core-owned interfaces cross the boundary. Owner: plan author confirms final signatures; default stands if unchallenged.
- `docs/adding-a-provider.md` seam-rule update in the same change — PROPOSED default: yes, small update to step 8 in the same PR: name the `RegisterHooks` seam and extend the existing "keep Pulumi SDKs out of `internal/cli`" wording with "non-test code under `cli/` and `internal/cli/` imports zero `internal/cloud/*`". Rationale: current wording covers Pulumi only, not direct provider imports. Owner: implementer; human page goes through humanizer then remove-ai-marks per repo rules.
