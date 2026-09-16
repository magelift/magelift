---
status: done
slug: sdk-module-extract
intent: intent.md
---

# Spec: SDK module extract (Option A, user-directed 2026-09-16)

Revision note: the accepted spec specified module path
`github.com/magelift/magelift/sdk/v1`. That path is illegal in Go — major
version suffixes are only allowed for v2 or later (`go mod init .../foo/v1`
fails; a require on such a path breaks all go commands with `invalid module
path`, verified empirically on toolchain go1.27.1). The user directed the
cleanest pre-release fix (Option A): module `github.com/magelift/magelift/sdk`
rooted at `sdk/`, code moved from `sdk/v1/*.go`, package renamed `v1` to
`sdk`, all importers rewritten once. Zero downstream breakage cost: no stable
release exists yet. This spec supersedes the module-path, tag-scheme, and
zero-churn requirements below; all mechanics, gates, and lockstep requirements
carry over with adjusted paths.

## Requirements

### Requirement: SDK owns its module files

`sdk` SHALL be an independent Go module rooted at `sdk/` with module path
`github.com/magelift/magelift/sdk`.

#### Scenario: module files exist and resolve

- **WHEN** the change is merged
- **THEN** `sdk/go.mod` exists with first line `module github.com/magelift/magelift/sdk`
- **AND** `sdk/go.sum` exists and is committed (empty: stdlib-only, `go mod tidy` creates no sums)
- **AND** `go list -m github.com/magelift/magelift/sdk` succeeds from `sdk/` with `GOWORK=off`.

#### Scenario: go directive mirrors the root toolchain

- **WHEN** the change is merged
- **THEN** `sdk/go.mod` contains `go 1.27.0`
- **AND** `sdk/go.mod` contains `toolchain go1.27.1` matching root `go.mod`.

### Requirement: SDK code lives at the module root as package sdk

All 37 Go files SHALL move from `sdk/v1/*.go` to `sdk/*.go` with the package
clause renamed `v1` to `sdk` and no other content change.

#### Scenario: move is path plus package clause only

- **WHEN** the change is merged
- **THEN** `ls sdk/*.go | wc -l` returns `37`
- **AND** `test ! -e sdk/v1` exits 0
- **AND** `git diff -M <base> HEAD -- sdk/ | grep -E '^[-+]' | grep -vE '^[-+]{3}' | grep -vE '^[-+]\s*package (v1|sdk)$'` returns empty (rename detection shows only the package-clause line changed per file).

#### Scenario: package clause is sdk everywhere

- **WHEN** the change is merged
- **THEN** `grep -rh '^package ' sdk/*.go | sort -u` returns exactly `package sdk`.

### Requirement: importers rewritten to the new path

Every Go import of `github.com/magelift/magelift/sdk/v1` SHALL become
`github.com/magelift/magelift/sdk`, with redundant aliases dropped and the 11
`v1.`-qualified files re-qualified to `sdk.`.

#### Scenario: old import prefix gone from live code

- **WHEN** the change is merged
- **THEN** `grep -rn "magelift/magelift/sdk/v1" --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty.

#### Scenario: new import count matches the pre-change count

- **WHEN** the change is merged
- **THEN** `grep -rn "magelift/magelift/sdk" --include='*.go' internal/ cli/ cmd/ examples/ sdk/ | wc -l` returns the pre-change count recorded before edits (343 import lines; no file gained or lost an SDK import).

#### Scenario: no stale qualifiers or aliases

- **WHEN** the change is merged
- **THEN** `grep -rn '[^a-zA-Z0-9_]v1\.' --include='*.go' internal/ cli/ cmd/ examples/ sdk/ | grep -v '_test.go.*otlp\|cockpit\|corev1\|metav1'` returns empty (no `v1.` SDK qualifiers remain; third-party `*v1` packages untouched)
- **AND** `grep -rn 'sdk "github.com/magelift/magelift/sdk"' --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty (no redundant alias identical to the package name).

### Requirement: workspace wires local development

The repo root SHALL provide a committed `go.work` workspace joining the root
module and the SDK module.

#### Scenario: workspace content

- **WHEN** the change is merged
- **THEN** `go.work` contains `go 1.27.0`
- **AND** `go.work` contains `use .`
- **AND** `go.work` contains `use ./sdk`.

#### Scenario: root resolves SDK through the workspace

- **WHEN** at repo root with default `GOWORK` mode
- **THEN** `go list -m github.com/magelift/magelift/sdk` succeeds
- **AND** `go build ./cli/...` succeeds
- **AND** `go vet ./internal/platform/` succeeds.

### Requirement: root requires the SDK without a local replace

Root `go.mod` SHALL declare a versioned `require` on the SDK module and SHALL NOT contain a `replace` pointing at `./sdk`. The require lands in the
post-tag follow-up commit (plan group 5), NOT in the implementation PR: the Go
toolchain must load the required version's go.mod to complete the module graph
even in workspace mode (verified empirically: any graph command fails with
`unknown revision` until the tag exists; the workspace resolves packages but
not the graph). Pre-tag, workspace-only resolution carries all builds, vets,
tests, and codegen.

#### Scenario: require present, replace absent (as of the 5.2 follow-up)

- **WHEN** the 5.2 follow-up commit exists
- **THEN** root `go.mod` requires `github.com/magelift/magelift/sdk v1.0.0-rc.1` (or a later lockstep version, see lockstep requirement; canonical block form — assert via `go mod edit -json`, not line shape)
- **AND** `go.mod` contains no line matching `replace github.com/magelift/magelift/sdk` (asserted in the implementation PR and the follow-up alike).

#### Scenario: custom-cli builds via workspace

- **WHEN** the change is merged
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli` succeeds.

### Requirement: SDK keeps zero dependencies and zero internal imports

`sdk` (non-test and test files) SHALL import only the Go standard library, enforced by construction via the module boundary.

#### Scenario: dependency purity by construction

- **WHEN** in `sdk/` with `GOWORK=off`
- **THEN** `go list -deps .` output, excluding the module's own path (the
  first line is always the package itself), contains no line starting with
  `github.com/`
- **AND** the same filtered output contains no line starting with `golang.org/`
- **AND** the same filtered output contains no line starting with `go.`
- **AND** the same filtered output contains no line containing `magelift/internal/`.

#### Scenario: tidy is a no-op in the SDK module

- **WHEN** in `sdk/`
- **THEN** `go mod tidy` exits 0
- **AND** `git diff --exit-code -- go.mod go.sum` exits 0.

### Requirement: nested-module tag scheme is documented and proven

SDK releases SHALL use nested-module tags of shape `sdk/vX.Y.Z`, documented in exactly one canonical doc, with at least one such tag proving the workflow.

#### Scenario: scheme documented

- **WHEN** the change is merged
- **THEN** `docs/adding-a-provider.md` contains the literal `sdk/vX.Y.Z` in its SDK pinning section.

#### Scenario: proof tag exists and resolves via the proxy

- **WHEN** the proof tag (default `sdk/v1.0.0-rc.1`) is pushed
- **THEN** `GOWORK=off GOPROXY=https://proxy.golang.org go list -m github.com/magelift/magelift/sdk@v1.0.0-rc.1` succeeds from a scratch module outside the repo.

### Requirement: SDK tags never trigger CLI releases

Pushing an `sdk/v*` tag SHALL NOT trigger the CLI release, container-image release, or any binary publish workflow.

#### Scenario: release trigger unaffected

- **WHEN** tag `sdk/v9.9.9-test` is pushed
- **THEN** `.github/workflows/release.yml` (trigger `tags: ['v*']`) does not start a run
- **AND** `.github/workflows/images.yml` (trigger `tags: ['v*']`) does not start a run (glob `v*` anchors at the start of the ref name; `sdk/v9.9.9-test` starts with `s`).

#### Scenario: GoReleaser config untouched by the SDK

- **WHEN** the change is merged
- **THEN** `.goreleaser.yaml` still defines exactly the two builds `magelift` (`./cmd/magelift`) and `magelift-provider-gcp` (`./cmd/magelift-provider-gcp`)
- **AND** `.goreleaser.yaml` contains no `sdk` build entry.

### Requirement: CI gates the SDK module independently

`.github/workflows/ci.yml` SHALL run an SDK-only gate that proves the SDK module builds, vets, tests, and scans standalone, and that gate SHALL block merges via the aggregator.

#### Scenario: path filters cover the new module files

- **WHEN** the change is merged
- **THEN** the `go` paths-filter in `.github/workflows/ci.yml` lists `sdk/go.mod`
- **AND** it lists `sdk/go.sum`
- **AND** it lists `go.work`
- **AND** it lists `go.work.sum`.

#### Scenario: SDK gate runs standalone and blocks merges

- **WHEN** the change is merged
- **THEN** `.github/workflows/ci.yml` defines job `sdk-verify` whose steps run `GOWORK=off go build ./...` and `GOWORK=off go vet ./...` and `GOWORK=off go test -race ./...` with `working-directory: sdk`
- **AND** the `result` job (`CI passed`) lists `sdk-verify` in its `needs:` array.

#### Scenario: SDK-only change exercises only the SDK gate plus aggregator

- **WHEN** a PR touches only `sdk/topology.go`
- **THEN** the `sdk-verify` job runs
- **AND** the `result` job (`CI passed`) succeeds.

### Requirement: codegen resolves through the workspace unchanged

All codegen entry points SHALL keep working with no logic changes, resolving the SDK via the workspace.

#### Scenario: generate checks stay green

- **WHEN** the change is merged
- **THEN** `go run ./cmd/genconfig --check` exits 0
- **AND** `go run ./cmd/gendocs --check` exits 0
- **AND** `go run ./cmd/gencertdocs --check` exits 0 (note: `cmd/gencertdocs/main.go` imports the SDK directly; `cmd/genconfig` reaches it not at all via `internal/config`; `cmd/gendocs` reaches it transitively via `internal/cli`).

### Requirement: custom-cli story stays workspace-resolved for v1

`examples/custom-cli` SHALL keep building unchanged via the workspace in this change (modulo the mechanical import-line rewrite); SDK pinning SHALL be proven by `examples/custom-extension-contract` instead.

#### Scenario: custom-cli builds via workspace with rewritten imports

- **WHEN** the change is merged
- **THEN** `examples/custom-cli/main.go` still imports `github.com/magelift/magelift/cli`
- **AND** `examples/custom-cli/stub_module.go` imports `github.com/magelift/magelift/sdk`
- **AND** `GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli` succeeds (it cannot pin SDK-only because `cli` stays in the root module).

#### Scenario: contract example proves proxy pinning after the first tag

- **WHEN** the proof tag (default `sdk/v1.0.0-rc.1`) exists
- **THEN** `GOWORK=off go build ./examples/custom-extension-contract` succeeds using the proxied SDK version from root `go.mod`
- **AND** `go list -deps ./examples/custom-extension-contract | rg 'magelift/internal/'` returns no match.

### Requirement: no SDK API break

The extract SHALL NOT change any exported SDK symbol, signature, or behavior. The package rename (`v1` to `sdk`) and import-path change are the only API-surface deltas, acceptable pre-release with zero downstream consumers.

#### Scenario: existing SDK tests green standalone

- **WHEN** in `sdk/` with `GOWORK=off`
- **THEN** `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./... -count=1` passes (all 37 files, as merged).

#### Scenario: public surface diff is package-name-only

- **WHEN** the change is merged
- **THEN** `go doc -all` over the SDK before the move (package `v1`) and after (package `sdk`) is identical after normalizing the package name and import prefix (mechanical proof: diff the two dumps with `package v1`→`package sdk` and `sdk/v1`→`sdk` applied; expect empty).

### Requirement: lockstep versioning for v1

For v1 the SDK version SHALL equal the CLI version numerically; this change proves mechanics only and ships no independent SDK release line.

#### Scenario: lockstep defined operationally

- **WHEN** CLI tag `v1.0.0-rc.1` and SDK tag `sdk/v1.0.0-rc.1` both exist
- **THEN** they point at commits with identical `sdk/**.go` trees
- **AND** root `go.mod` requires `github.com/magelift/magelift/sdk v1.0.0-rc.1`
- **AND** `.release-please-manifest.json` still contains only the single `.` entry (no release-please SDK package; SDK tags are cut manually lockstep until post-v1 automation).

#### Scenario: full local gate stays green

- **WHEN** the change is merged
- **THEN** `make verify` exits 0.

### Requirement: live docs name the new SDK home

Every live (non-historical) `sdk/v1` path or import reference SHALL move to `sdk`, so docs, skills, and READMEs describe the tree as it is.

#### Scenario: live sweep clean

- **WHEN** the change is merged
- **THEN** `grep -rn 'sdk/v1' AGENTS.md CONTRIBUTING.md tests/README.md contrib/skills/magelift-provider/SKILL.md docs/adding-a-provider.md docs/adr/0003-portable-contracts-vs-topology.md docs/adr/0004-ports-and-adapters.md docs/architecture.md docs/versioning.md examples/custom-cli/README.md` returns empty
- **AND** `grep -rn 'magelift/magelift/sdk/v1' --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty (no stale import strings anywhere in live code).

## Design

### Why the module is `.../sdk`, not `.../sdk/v1` (disproof record)

Go forbids `/v0` and `/v1` module-path suffixes (major suffixes only for v2+;
vendored `golang-dependency-management/references/versioning.md`: "The `v0`
and `v1` versions have no suffix"). Empirically: `go mod init
github.com/example/foo/v1` fails, and a require on such a path breaks every go
command with `invalid module path` (observed 2026-09-16, toolchain go1.27.1).
The originally decided `.../sdk/v1` module path can therefore never exist.
Since no stable release exists, the cleanest fix — user-directed — is a
one-time rename to the textbook shape rather than a compatibility shim:

- Module: `github.com/magelift/magelift/sdk`, rooted at `sdk/`
  (`sdk/go.mod`, `sdk/go.sum`). Tags: `sdk/vX.Y.Z` (nested-module convention:
  tag prefix follows the module directory).
- Package: `sdk` (clause renamed in all 37 files; packages match their
  directory per `golang-project-layout` and Go convention).
- Imports: `github.com/magelift/magelift/sdk` (343 lines rewritten once;
  `sdk.` qualifiers unchanged for the 332 already-aliased files).
- Rejected alternative (minimal churn, permanent oddity): module `.../sdk`
  rooted at `sdk/` with code kept in `sdk/v1/` as package `v1`. It preserves
  import strings but fossilizes a `v1/` directory whose meaning (module
  version? package version?) drifts from the module version forever, and it
  traps future v2 (an import ending `/sdk/v2` would demand a nonexistent
  module). Wrong base for a pre-release project optimizing for maintainability.

### New module files

`sdk/go.mod` (new, minimal — zero requires expected):

```text
module github.com/magelift/magelift/sdk

go 1.27.0

toolchain go1.27.1
```

Rationale: `go 1.27.0` + `toolchain go1.27.1` mirror root `go.mod` exactly so
`setup-go` (`go-version-file: go.mod`) and local builds use one toolchain. No
`require` block: all 37 files import stdlib only, zero
`magelift/magelift/(internal|cli|cmd|sdk)` imports (the only `github.com` hit
in tests is a string literal in `validation_test.go`).

`sdk/go.sum` (new, committed): empty — `go mod tidy` creates no sums for a
zero-dependency module, so the file is touched into existence to satisfy the
exists-and-committed requirement and CI hash keys; tidy is then a proven
no-op. Per `golang-dependency-management`: `go.sum` is committed;
`govulncheck` runs in the SDK gate (trivially clean today, locks the invariant).

### `go.work` (new, committed)

```text
go 1.27.0

toolchain go1.27.1

use (
	.
	./sdk
)
```

`go.work.sum`: NOT committed — added to `.gitignore` per current workspace
guidance (`golang-dependency-management/references/workspaces.md`: workspaces
are for local development; `go.work.sum` is a regenerable local artifact).
With a zero-dep SDK, `go work sync` generates no file at all; CI
`hashFiles(...)` entries tolerate its absence. `go.work` itself stays
committed: the monorepo pattern is committed `go.work` + separate modules, and
an ignored workspace would make every fresh clone resolve the SDK from the
proxy and break pre-tag development.

### Root `go.mod` change: require lands post-tag, never replace

The versioned require (`github.com/magelift/magelift/sdk v1.0.0-rc.1`, or the
lockstep version current at tag time) lands in the 5.2 follow-up commit via
`go mod edit -require=`, once the proof tag exists on the proxy. It MUST NOT
land in the implementation PR: the toolchain loads the required version's
go.mod to complete the module graph even in workspace mode, so any require on
the still-untagged version breaks every graph command (build/vet/test/tidy)
with `unknown revision` — verified empirically. Conversely, root tidy without
the require also fails pre-tag (`no matching versions for query "latest"`),
so pre-tag `go.mod`/`go.sum` stay byte-identical and the first successful root
tidy runs in the follow-up. Accept whatever canonical block placement that
tidy chooses (Go 1.27 two-block layout); assert presence via
`go mod edit -json`, never via single-line shape.

Rationale for require-without-replace over the alternatives (unchanged from the
original design):

- **Committed `replace ./sdk` (rejected):** breaks all downstream consumers
  and masks version skew; would have to be reverted for the first real tag.
- **Workspace-only with no require (rejected):** any `GOWORK=off` build fails
  to resolve. The require line is what makes the SDK a real dependency.
- **Require + committed workspace (chosen):** local/CI builds resolve to
  `./sdk` via `go.work`; registry/proxy builds resolve to the tagged version.
  Known wart: until the proof tag lands, `GOWORK=off` root builds fail (version
  not yet on the proxy). Document that wart in the implementation PR; it
  disappears the moment `sdk/v1.0.0-rc.1` is pushed.

### Move + rewrite mechanics (Option A)

Order matters: move and rewrite FIRST inside the root module (every step
compiles and tests green), then split the module. Sequence:

1. `git mv sdk/v1/*.go sdk/` (37 files; `sdk/v1/` vanishes).
2. `package v1` → `package sdk` in all 37 (test files are all internal
   `package v1`, none external `v1_test` — no `_test` suffix handling).
3. Import rewrite across the 343 importer lines:
   - 332 × `sdk "github.com/magelift/magelift/sdk/v1"` → bare
     `"github.com/magelift/magelift/sdk"` (drop the now-redundant alias;
     `sdk.` qualifiers byte-identical, so use sites untouched; safe by
     construction — no file can hold a colliding `sdk` ident while the alias
     exists).
   - 2 × `v1 "..."` + 9 bare `"..."` → bare new path plus `v1.` → `sdk.`
     qualifier rewrites (all 77 `v1` tokens in those 11 files verified to be
     import lines or `v1.Exported` qualifiers; no `v1` variables; no `sdk`
     idents to collide with; the 4 `otlp/v1` URL literals untouched by the
     dot-requiring pattern).
   - 3 × Go comment mentions (`sdk/v1` prose) → `sdk`.
   - Method: `gofmt -r` for the path strings (plan-verified gofmt-clean
     inputs), explicit alias-drop + qualifier passes, compiler + suites as the
     net. No `internal/` imports are added or removed anywhere.
4. Capture the `go doc -all` export dump before/after for the package-name-only
   proof (normalize and diff; expect empty).

Docs/skills/READMEs (15 lines, 10 files): mechanical `sdk/v1` → `sdk` path
swaps in `AGENTS.md` (Map), `contrib/skills/magelift-provider/SKILL.md`,
`CONTRIBUTING.md`, `docs/adding-a-provider.md` (×2 incl. the order-16 rule
table), `docs/adr/0003-*.md` (×2), `docs/adr/0004-*.md`, `docs/architecture.md`
(×2), `docs/versioning.md` (×2), `examples/custom-cli/README.md`,
`tests/README.md`, plus the new pinning subsection. Historical records
(`intent/`, sealed evidence) untouched by design. Touched human pages go
through humanizer, then remove-ai-marks, per repo rules.

### Critical Go behavior shaping the design: nested-module `./...` exclusion

Once `sdk/` is its own module, root `./...` patterns **exclude** it. That silently descopes:

- `Makefile`: `test` (`go test -race ./...`), `lint` (`golangci-lint run ./...`), `license-check` (`go-licenses check ./...`), `pulumi-mock-test` (unaffected path-wise, still root-scoped — fine).
- CI `go-verify`: `go test -race ./...`, `govulncheck ./...`, `go-licenses check ./...`; CI `lint` (`args: ./...`).
- `fmt-check`/`fmt` use `find . -name '*.go'` — still cover `sdk/`; no change needed.

Hence the dedicated `sdk-verify` job is load-bearing, not cosmetic. Options were (a) new job (chosen), (b) extending `go-verify` with `cd sdk` steps (couples failure signals; SDK-only PRs would still pay the full root suite). (a) gives a clean per-module signal and matches `golang-continuous-integration` per-module gates.

Proposed `Makefile` addition (serial, honoring exported `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`):

```make
sdk-test: ## Run the SDK module suite standalone (no workspace)
	cd sdk && GOWORK=off go test -race ./... -count=1
```

Wire `sdk-test` into `verify` prerequisites alongside `test`. Keep `extension-test` (`scripts/custom-extension-clean-room.sh`) as-is — it builds `./examples/custom-extension-contract` with empty caches through the workspace and asserts no `magelift/internal/` in `go list -deps`; after the split that assertion is enforced by the module boundary too.

### CI filter/job changes (`ci.yml`)

1. `go` filter: append `sdk/go.mod`, `sdk/go.sum`, `go.work`, `go.work.sum`. (`**/*.go` already covers `sdk/*.go`; only the four module workspace files are newly uncovered.)
2. New `changes` output `sdk` with its own filter block:
   ```yaml
   sdk:
     - 'sdk/**/*.go'
     - 'sdk/go.mod'
     - 'sdk/go.sum'
     - 'go.work'
     - 'go.work.sum'
   ```
   plus the same `workflow_dispatch inputs.all` OR-clause as the other outputs.
3. New job `sdk-verify` (runs on `needs.changes.outputs.sdk == 'true'`, NOT skipped for Dependabot — SDK has zero deps today but the gate is cheap):
   - `actions/checkout` (persist-credentials false), `actions/setup-go` (keep `go-version-file: go.mod`; do not switch to `go.work` — unverified `setup-go` behavior), module cache restore keyed on `hashFiles('go.sum', 'sdk/go.sum', 'go.work.sum')`.
   - Steps (each `working-directory: sdk`, `env: GOWORK: off` to prove standalone hermeticity): `go build ./...`, `go vet ./...`, `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1`, `go mod tidy && git diff --exit-code -- go.mod go.sum`, `govulncheck ./...` (via `golang/govulncheck-action` with `working-directory` if the pinned action supports it, else fallback pinned `go run golang.org/x/vuln/cmd/govulncheck@<hash> ./...`), plus a `golangci-lint` step from `sdk/` (config found via parent traversal of root `.golangci.yml`).
   - No `tool` directives introduced: repo pins tools via `go run ...@version`/actions today; migrating to `tool` blocks is unrequested toolchain churn (intent out-of-scope), so the SDK gate mirrors the existing repo pattern.
4. `cache-prime`: extend cache keys from `hashFiles('go.sum')` to `hashFiles('go.sum', 'sdk/go.sum', 'go.work.sum')` and keep `go mod download` (workspace-aware: warms both modules).
5. `result` aggregator: add `sdk-verify` to `needs:`. Without this the gate is advisory — the single required check is `CI passed`.

### Tag/branch protection notes

- `release.yml` trigger `tags: ['v*']` and `images.yml` trigger `tags: ['v*']` need NO change: GitHub tag globs match the full ref name, and `sdk/vX.Y.Z` starts with `s`, so SDK tags cannot start either workflow. The requirement scenario pushing `sdk/v9.9.9-test` is the regression proof (delete the tag after observing zero runs; or document the dry-run reasoning if the repo forbids test tags — prefer the real push-then-delete, it is the only honest trigger test).
- `release-please.yml` triggers on `main` push only and manages only the `.` package — unaffected by SDK tags. No `release-please-config.json` change: SDK tags are cut manually, lockstep, until post-v1 automation (independent releases are explicitly out of scope per `v1-stable-cut`).
- GoReleaser: no change (libraries ship no binaries; per `golang-continuous-integration` libraries need at most a notes-only Release). SDK distribution is the Go module proxy reacting to the git tag. Optional visibility: `gh release create sdk/vX.Y.Z --title ... --notes ...` with no artifacts — notes-only GitHub Release. Default: do it for the proof tag (discoverability), skip binary uploads.
- Branch protection: no new rules; the existing required `CI passed` check absorbs `sdk-verify` via the aggregator.

### custom-cli migration plan

- **This change (default):** no `examples/custom-cli` logic edits — only the mechanical SDK import-line rewrite shared with all importers. Rationale: `main.go` imports root-module `cli`, so the example cannot become an SDK-only pinned consumer until/unless `cli` itself is extracted (out of scope). It keeps building through `go.work`, which is exactly what the local-dev story promises.
- **Pinning proof (this change):** `examples/custom-extension-contract` imports ONLY the SDK, so post-tag `GOWORK=off go build ./examples/custom-extension-contract` proves proxy resolution end-to-end. Add that as a manual verification step tied to the proof tag (and optionally a CI step conditioned on tag presence — default manual, to avoid CI brittleness around tag timing).
- **Docs:** `docs/adding-a-provider.md` gains an SDK-pinning subsection (`require github.com/magelift/magelift/sdk vX.Y.Z`, no replace, tags `sdk/vX.Y.Z`). `magelift-extend` skill: no change (boundary wording uses `sdk.Module`, unaffected by the path move). ADR: no new ADR unless review decides the public boundary changed — pre-release, the import path move breaks zero consumers.

### Rollback

Single-commit revert restores the monolith: delete `go.work` (and `.gitignore` line), delete `sdk/go.mod` + `sdk/go.sum`, `git mv sdk/*.go sdk/v1/` back with `package sdk` → `package v1`, revert the 343 importer lines, revert the root `go.mod` require line (and any `go.sum` churn), revert `ci.yml` filter/job/`needs` edits, revert the `Makefile` `sdk-test` wiring and the docs path swaps. If the proof tag already propagated to the Go proxy, leave it (immutable, harmless); do NOT force-delete published module versions.

## Gotchas / policy flags

- **Serial-build discipline:** the split adds a second `go test -race ./...` (root + SDK). Both run under the existing Makefile exports (`GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`, `.NOTPARALLEL`). Never run both suites in parallel from an IDE session; `make verify` and CI run them sequentially. The SDK suite is small (stdlib-only, fast).
- **`go mod tidy` in two modules:** root tidy (`go mod tidy` at root, workspace-aware) and SDK tidy (`GOWORK=off go mod tidy` in `sdk/`) are separate commands with separate effects. Forgetting the SDK tidy lets `sdk/go.sum` drift silently because root `./...` no longer covers it. CI `sdk-verify` enforces SDK tidiness.
- **Nested-module `./...` blindness (the big one):** after the split, root `go test`, `go vet`, `golangci-lint`, `govulncheck`, and `go-licenses` all silently skip `sdk/`. Any future "all Go" command must be audited for the second module. `grep -rn 'go test\|go vet\|golangci\|govulncheck\|go-licenses' Makefile .github/ scripts/` is the audit command at implementation time.
- **Dependabot misses `sdk/` (flagged, small fix in scope):** `.github/dependabot.yml` has one `gomod` entry (`directory: /`). Dependabot does not follow nested modules, so `sdk/go.mod` would never get bump PRs. Zero deps today means zero immediate exposure, but add the second entry now (split forces it):
  ```yaml
  - package-ecosystem: gomod
    directory: /sdk
    schedule: { interval: weekly }
    # …same cooldown/labels/groups as the root entry…
  ```
  Full Renovate migration stays out of scope per the intent (toolchain migrations excluded unless forced; Dependabot multi-entry covers the forced part).
- **`v*` vs `sdk/v*` trigger collision risk:** none under current globs, but the safety is implicit (glob anchoring). If anyone later broadens a release trigger to `tags: ['**/v*']` or `'*'`, SDK tags would start building CLI artifacts. The `sdk/v9.9.9-test` push-then-delete scenario is the standing regression test; consider a comment above each `tags: ['v*']` trigger noting the SDK-tag exclusion.
- **Single-version v1 promise:** this change proves multi-module mechanics; it does NOT ship an independent SDK release line. No SDK-only version numbers, no SDK changelog process, no release-please SDK package until post-v1 (`v1-stable-cut` constraint). The proof tag `sdk/v1.0.0-rc.1` is lockstep with the CLI RC, not a separate release.
- **No runtime loader change:** no `plugin.Open`, no new subprocess boundary, no provider-host protocol change. Compile-time imports only; `internal/providerhost` gRPC surface untouched.
- **`GOWORK=off` root builds fail until the proof tag lands:** the only ordering wart (require version not yet on the proxy). Implementation order: merge code → push `sdk/vX.Y.Z` from the merge commit → verify proxy resolution → cut/confirm CLI tag lockstep. Document in the PR; do not paper over with a temporary committed `replace`.
- **Rewrite blast radius is mechanical but wide:** 343 import lines plus 37 package clauses plus 11 qualifier files plus 15 doc lines. Every rewrite step re-runs `gofmt -l` (expect empty), the affected suites, and ends with the stale-prefix sweeps. The compiler is the final net: any missed qualifier or alias is a build error, not a silent behavior change.

## Open questions carried forward

- **Module path — DECIDED (user, 2026-09-16, supersedes the original submodule decision): `github.com/magelift/magelift/sdk` rooted at `sdk/`.** The original `.../sdk/v1` decision was disproven (Go rejects `/v1` module suffixes). Rationale locked in: textbook v1 module shape, zero downstream cost pre-release, best long-term base.
- **Tag scheme — DECIDED (user, 2026-09-16, supersedes `sdk/v1/vX.Y.Z`): nested `sdk/vX.Y.Z`.** Go nested-module convention for a module rooted at `sdk/`; proxy-resolvable; provably disjoint from the `v*` CLI release trigger.
- **GoReleaser change needed, or tags alone? — PROPOSE: tags alone (default).** Owner: implementer to confirm at build time. Libraries need no binaries; `.goreleaser.yaml` stays at the two existing builds; SDK ships via git tag + module proxy with an optional notes-only `gh release create sdk/vX.Y.Z`.
- **custom-cli switches now or after the first tag? — PROPOSE: stays workspace-resolved; no switch in this change (default).** Owner: implementer. Rationale: `examples/custom-cli` imports root-module `cli`, so it cannot pin SDK-only regardless of tags; it gets the mechanical import rewrite like every importer and keeps building through `go.work`. Pinning is proven by `examples/custom-extension-contract` (SDK-only imports) with `GOWORK=off` post-tag. Revisit only if `cli` is extracted (out of scope).
