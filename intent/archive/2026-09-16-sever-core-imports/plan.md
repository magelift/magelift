---
status: done
slug: sever-core-imports
spec: spec.md
---

# Plan: sever core imports

## Files that change

### Hooks ports (internal/cli)

- **Edit** `internal/cli/root.go` — add `NewCleanupProvider` + `MediaEndpoint` to `Hooks` (line 40), add `mediaEndpoint` to `options` (line 63), thread all six hooks in `newCommandWithHooks` (line 195).
- **Edit** `internal/cli/cleanup.go` — drop 3 `internal/cloud` imports (lines 15-17); convert `defaultCleanupProvider` (line 205) to hook dispatch on `*options`; delete moved OVH/Scaleway/GCP-fallback bodies after 2.1 pastes them.
- **Edit** `internal/cli/env.go` — drop `internal/cloud/aws/endpoint` import (line 17); resolve emulator endpoint via injected `MediaEndpoint` hook in `newMediaS3Client` (line 766); nil hook means `""`.

### Aggregator (internal/registry)

- **New** `internal/registry/hooks.go` — `RegisterHooks() internalcli.Hooks` owning all provider hook construction (moved bodies verbatim); sole core-adjacent importer of the 8 provider hook packages.
- **Edit** `internal/registry/registry.go` — doc-only pointer to `hooks.go`; `NewDefault` body and 6 `Module{}` entries (lines 17-22) unchanged.

### Entrypoints (cmd/magelift, cli/)

- **Edit** `cli/cli.go` — add sibling `NewWithExtensionsAndHooks`; rewire `NewWithExtensions` to delegate hooks to `registry.RegisterHooks()`; delete 5 cloud imports (lines 12-16), `firstPartyHooks` (line 54), both glue types (lines 71, 85).
- **Edit** `cmd/magelift/main.go` — explicit `registry.NewDefault()` + `registry.RegisterHooks()` + sibling constructor; no `init`, no behavior change.

### Tests

- **New** `internal/registry/hooks_test.go` — `RegisterHooks` populates all six hooks non-nil; no credentialed calls; error-string spot checks.
- **Edit** `internal/cli/root_test.go` — add `NewWithModulesAndHooks` threading test for `NewCleanupProvider` + `MediaEndpoint` (nil-safe zero value, override dispatch).
- Checked, no edit expected (options seams, zero `Hooks` literals — must keep compiling): `internal/cli/cleanup_test.go` (`o.newCleanupProvider`), `internal/cli/lifecycle_test.go` (`o.newLeftoverBackupDestroyer`), `internal/cli/build_test.go` (`options{newComposerGCPSecrets}`), `internal/cli/env_media_sync_test.go` (`o.mediaSync`). No test file constructs `Hooks` directly.

### Docs

- **Edit** `docs/adding-a-provider.md` — step 8 (lines 64-68) names the `RegisterHooks` seam and extends the Pulumi wording with the zero-`internal/cloud/*` rule; humanizer, then remove-ai-marks.

Not changing (build locks): `examples/custom-cli/main.go`, `cmd/gendocs/main.go`, `internal/cli/sign.go` (line 154 is a comment-only provider mention; greps use the quoted import path so it stays safe).

## Order of work

- [x] 1.1 In `internal/cli/root.go`, extend `Hooks` with `NewCleanupProvider func(context.Context, sdk.CleanupLedger) (CleanupProvider, error)` and `MediaEndpoint func() (string, error)`; add `mediaEndpoint func() (string, error)` to `options` (`newCleanupProvider`/`newGCPCleanupProvider` fields already exist); thread `hooks.NewCleanupProvider` and `hooks.MediaEndpoint` in `newCommandWithHooks` alongside the existing four (lines 230-233); nil stays nil — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1` exits `0`.
- [x] 1.2 In `internal/cli/root_test.go`, add a threading test proving `NewWithModulesAndHooks` wires `NewCleanupProvider`/`MediaEndpoint` into `options` (zero `Hooks{}` leaves them nil; set hooks dispatch through `cleanupProviderFor` and the media client path) — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1` exits `0`.
- [x] 2.1 Create `internal/registry/hooks.go` with `func RegisterHooks() internalcli.Hooks`, copying bodies verbatim from the current tree (no rewording): `NewComposerSecrets` via `awssecrets.New` (`func New(ctx context.Context, region string) (*Resolver, error)`, `internal/cloud/aws/secrets/awssecrets.go:53`); `NewComposerGCPSecrets` via `gcpsecrets.NewStore` (`func NewStore(ctx context.Context) (*Store, error)`, `internal/cloud/gcp/secrets/secrets.go:39`) plus the `gcpComposerSecretAdapter` with `GetParameter` refusal `GCP Secret Manager does not resolve Parameter Store references`; `NewLeftoverBackupDestroyer` via `gcpstack.AsGCPPlanned` (`func AsGCPPlanned(planned platform.PlannedStack) (Planned, bool)`, `module.go:142`), `naming.CloudSQLInstance` (`func CloudSQLInstance(project, environment string) string`, `naming.go:12`), `gcpresilience.NewGCPCloudSQLNativeAPI` (`func NewGCPCloudSQLNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error)`, `native.go:278`); `NewGCPCleanupProvider` via the same native API (keep populated for seam continuity); `NewCleanupProvider` as the moved `defaultCleanupProvider` switch using `gcpresilience.NewCloudSQLAPI` (`func NewCloudSQLAPI(ctx context.Context) (CloudSQLAPI, error)`, `native_sql_sdk.go:26`), `ovhresilience.NewOVHClientFromProfile` (`func NewOVHClientFromProfile(profile string) (*ovh.Client, error)`, `ovh_profile.go:19`), `ovhresilience.NewOVHDatabaseNativeAPI` (`func NewOVHDatabaseNativeAPI(ctx context.Context, config NativeAPIConfig, database *ovh.Client) (*NativeAPI, error)`, `native_objects.go:171`), `resilience.NewScalewayDatabaseNativeAPI` (`func NewScalewayDatabaseNativeAPI(ctx context.Context, config NativeAPIConfig, options ...scw.ClientOption) (*NativeAPI, error)`, `native_database_sdk.go:27`); `MediaEndpoint` as `func() (string, error) { return awsendpoint.FromEnv() }` (`func FromEnv() (string, error)`, `endpoint.go:17`). Preserve verbatim: `GCP cleanup ledgers require a project`, `OVHcloud cleanup ledgers require profile, region, and project`, `Scaleway cleanup ledgers require profile, region, and project`, `create GCP Cloud SQL cleanup client: %w`, `create GCP Cloud SQL leftover backup client: %w`, `leftover Cloud SQL backup destroy requires a GCP planned stack`, `leftover Cloud SQL backup destroy requires a GCP project and instance name`, all four Scaleway profile errors, and the AWS loopback refusal (inside the provider package). No `init`, no globals — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/registry/ -count=1` exits `0`.
- [x] 2.2 Create `internal/registry/hooks_test.go` asserting all six `RegisterHooks()` fields are non-nil, `MediaEndpoint` returns `""` with unset env and surfaces the loopback-refusal error on a bad value, and no constructor is invoked at registration time (no credentials needed) — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/registry/ -count=1` exits `0`.
- [x] 2.3 In `internal/registry/registry.go`, add only a doc pointer to `hooks.go`; `NewDefault` body untouched — verify: `grep -rn 'func init(' cli/ internal/cli/ internal/registry/ --include='*.go' | grep -v '_test.go'` returns empty.
- [x] 3.1 In `cli/cli.go`, add the sibling constructor `func NewWithExtensionsAndHooks(hooks internalcli.Hooks, extensions ...sdk.Module) (*cobra.Command, error)` delegating to `registry.NewDefault` + `internalcli.NewWithModulesAndHooks` (chosen name mirrors `internalcli.NewWithModulesAndHooks` so public/private constructors read as a pair, with hooks first because the variadic extensions must stay last); `NewWithExtensions` keeps its exact signature in this step — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./cli/` exits `0`.
- [x] 3.2 In `internal/cli/cleanup.go`, convert `defaultCleanupProvider` to `func (o *options) defaultCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (cleanupProvider, error)` with dispatch body only (prefer `o.newCleanupProvider`, then `o.newGCPCleanupProvider` for `gcp` ledgers, else `cleanup.UnsupportedProvider`); simplify `cleanupProviderFor` to delegate to it; delete the moved OVH/Scaleway/GCP bodies; remove the 3 cloud imports plus `scw`/`credentials` imports if now unused (confirm via compile; `strings`/`time` stay — used elsewhere in the file) — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1` exits `0`.
- [x] 3.3 In `internal/cli/env.go`, convert `newMediaS3Client` to `func (o *options) newMediaS3Client(ctx context.Context, region string) (*s3.Client, error)` reading `o.mediaEndpoint` (nil hook yields `""`, i.e. normal AWS endpoints); keep the `invalid(err)` wrap on hook error and the rest of the body verbatim; update the call site (line 721); remove only the `awsendpoint` import (AWS SDK imports stay — not `internal/cloud`) — verify: `grep -rn '"github.com/magelift/magelift/internal/cloud' internal/cli/ --include='*.go' | grep -v '_test.go'` returns empty.
- [x] 3.4 In `cli/cli.go`, rewire `NewWithExtensions` to `return NewWithExtensionsAndHooks(registry.RegisterHooks(), extensions...)`; delete `firstPartyHooks`, `gcpComposerSecretAdapter`, `gcpCloudSQLLeftoverBackupDestroyer`, and the 5 cloud imports — verify: `grep -rn '"github.com/magelift/magelift/internal/cloud' cli/ --include='*.go' | grep -v '_test.go'` returns empty.
- [x] 3.5 In `cmd/magelift/main.go`, become the explicit registration site: `modules, err := registry.NewDefault()`, `hooks := registry.RegisterHooks()`, `command, err := cli.NewWithExtensionsAndHooks(hooks)`; keep error handling, `cli.Execute`, and `cli.ExitCode` behavior identical — verify: `grep -rn 'RegisterHooks' cmd/magelift/ --include='*.go'` returns non-empty.
- [x] 3.6 Confirm custom-binary compatibility after the rewire (check step, no code) — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./examples/custom-cli/` exits `0`.
- [x] 4.1 Update `docs/adding-a-provider.md` step 8 to name the `RegisterHooks` seam and extend `Keep Pulumi SDKs out of internal/cli` with `non-test code under cli/ and internal/cli/ imports zero internal/cloud/*`; run humanizer, then remove-ai-marks, per repo rules — verify: `make verify` exits `0`.

## Risks

- Custom-binary compat breaks if `NewWithExtensions` drifts in signature or default-hook behavior — check: `grep -rn 'func NewWithExtensions(extensions ...sdk.Module)' cli/ --include='*.go'` plus `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./examples/custom-cli/`.
- Error-string drift on move (cleanup validation, GCP adapter refusal, leftover-destroyer, Scaleway profile, endpoint loopback) — check: each verbatim string greps in `internal/registry/hooks.go` and both suites stay green.
- Test-seam churn if `options` hook field names/types change (`newCleanupProvider`, `newGCPCleanupProvider`, `newLeftoverBackupDestroyer`, `newComposerSecrets`, `newComposerGCPSecrets`, `mediaSync`) — check: only `root_test.go` gains a test; `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1` green with no other test edits.
- Secret-resolution regression if `secretRegion` (ARN parsing, stays in `internal/cli/build.go`) or the GCP adapter is altered — check: `grep -rn 'func secretRegion' internal/cli/ --include='*.go'` still present and `build_test.go` composer-credential cases green; no secret values in YAML/logs/evidence (reference-only).
- Gendocs link break if `internal/cli` gains Pulumi or cloud reachability (note: the new edge is `registry` → `internal/cli`, never the reverse; `gendocs` imports `internal/cli` only) — check: `grep -rn 'pulumi/pulumi/sdk' internal/cli/ --include='*.go' | grep -v '_test.go'` returns empty plus `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./cmd/gendocs/`.
- Import cycle from `internal/registry` importing `internal/cli` — check: `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./internal/registry/` (fails fast on a cycle; `internal/cli` and its deps import no `registry` today).
- Intended nil-hook behavior change on the zero-`Hooks{}` path (cleanup without hooks now returns `UnsupportedProvider` instead of constructing real clients; media client defaults to `""`) surprising callers — check: 1.2 threading test locks both fallbacks; full suites green.

## Proof

```sh
grep -rn '"github.com/magelift/magelift/internal/cloud' cli/ --include='*.go' | grep -v '_test.go'
grep -rn 'awssecrets\|gcpsecrets\|gcpresilience\|gcpstack\|gcp/naming' cli/ --include='*.go' | grep -v '_test.go'
grep -rn '"github.com/magelift/magelift/internal/cloud' internal/cli/ --include='*.go' | grep -v '_test.go'
go list -f '{{ join .Imports "\n" }}' ./internal/cli/ | grep 'internal/cloud'
grep -rn 'RegisterHooks' cmd/magelift/ --include='*.go'
grep -rn 'func init(' cli/ internal/cli/ internal/registry/ --include='*.go' | grep -v '_test.go'
grep -rn 'pulumi/pulumi/sdk' internal/cli/ --include='*.go' | grep -v '_test.go'
grep -c 'Module{' internal/registry/registry.go
grep -rn 'func NewDefault' internal/registry/ --include='*.go' | grep -v '_test.go'
grep -rn '"github.com/magelift/magelift/internal/cloud' sdk/v1/ --include='*.go' | grep -v '_test.go'
grep -rn '"github.com/magelift/magelift/internal/cloud' internal/config/ --include='*.go' | grep -v '_test.go'
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/registry/ -count=1
GOMAXPROCS=1 GOFLAGS=-p=1 go build ./examples/custom-cli/
GOMAXPROCS=1 GOFLAGS=-p=1 go build ./cmd/gendocs/
make verify
```

Expected: all `internal/cloud`, alias, `go list`, `init`, and Pulumi greps empty; `RegisterHooks` and `NewDefault` greps non-empty; `Module{` count `6`; `sdk/v1` and `internal/config` greps empty; both suites, both builds, and `make verify` exit `0`.
