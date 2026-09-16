---
status: done
slug: sdk-module-extract
spec: spec.md
---

# Plan: SDK module extract (Option A, user-directed 2026-09-16)

Revision note: rewritten for module `github.com/magelift/magelift/sdk` rooted
at `sdk/` (user decision; the original `.../sdk/v1` module path is illegal Go
— see spec Design). Move + rewrite land FIRST inside the root module (every
step compiles green), then the module split. Group 5 is user release action
only (AGENTS.md ask-first + goal-brief no-tag rule) and is NOT executed by the
implementer.

## Files that change

### Move (via `git mv`, then package-clause rename)

37 files `sdk/v1/*.go` → `sdk/*.go` (21 non-test + 16 `*_test.go`; all
`package v1`, none external `v1_test`). `sdk/v1/` vanishes. Package clause
becomes `package sdk` in all 37 — the only content change inside the moved
files. No `testdata/` exists under `sdk/v1/`.

### Rewrite (import lines + qualifiers + comments, mechanical)

- 332 files: `sdk "github.com/magelift/magelift/sdk/v1"` → bare
  `"github.com/magelift/magelift/sdk"` (drop the now-redundant alias;
  `sdk.` qualifiers byte-identical, use sites untouched; safe by
  construction — no file can hold a colliding `sdk` ident while the alias
  compiles).
- 9 bare-import files + 2 `v1`-aliased files (11 total, listed below): import
  line → bare new path, plus `v1.` → `sdk.` qualifier rewrites. Verified
  safe: all 77 `v1` tokens in these files are import lines or `v1.Exported`
  qualifiers (no `v1` variables; no `sdk` idents to collide with); the 4
  `otlp/v1` URL literals are untouched by the dot-requiring pattern.
- 3 Go comment mentions → `sdk`: `internal/cloud/aws/resilience/adapter.go`,
  `internal/cloud/gcp/observability/component.go`,
  `internal/certification/cleanup.go`.

The 11 qualifier files: `internal/cli/extensions.go`,
`internal/cloud/scaleway/observability/client_test.go`,
`internal/external/newrelic/integration.go`,
`internal/cloud/aws/observability/client_test.go`,
`internal/cloud/aws/observability/client.go`,
`internal/cloud/aws/stack/config_test.go`,
`internal/cloud/gcp/observability/client_test.go`,
`internal/cloud/gcp/observability/client.go`,
`cmd/gcp-observability-acceptance/main.go`,
`cmd/scaleway-observability-acceptance/main.go`,
`cmd/aws-cloudwatch-acceptance/main.go`.

Pre-change count to beat: 343 lines matching
`magelift/magelift/sdk/v1"` across `internal/ cli/ cmd/ examples/`
(recorded 2026-09-16; the implementer re-records before editing in case the
tree moved).

### New

- `sdk/go.mod` (new) — module `github.com/magelift/magelift/sdk`, `go 1.27.0`,
  `toolchain go1.27.1`, zero requires.
- `sdk/go.sum` (new) — empty, committed (tidy creates no sums for zero deps;
  touch into existence, then prove tidy no-op).
- `sdk/LICENSE` (new) — byte-identical copy of root Apache-2.0 `LICENSE`.
  Required: `go-licenses` resolves licenses per dependency module directory,
  and SDK consumers (plus pkg.go.dev) need the grant in-module. No
  `sdk/NOTICE`: the root NOTICE's operative content is third-party
  attribution, and the SDK has zero third-party code (revisit if a
  dependency ever lands).
- `go.work` (new) — `go 1.27.0`, `toolchain go1.27.1`, `use (` `.` + `./sdk` `)`.

### Edit: Go-adjacent

- `go.mod`/`go.sum` (NO CHANGE in the implementation PR) — the versioned
  require (`github.com/magelift/magelift/sdk v1.0.0-rc.1`, or later lockstep
  version) lands in the post-tag 5.2 follow-up commit, when `go mod tidy` can
  complete against the published tag. Pre-tag, any require (present or absent)
  breaks either graph loads or tidy; workspace-only resolution is the coherent
  pre-tag steady state.
- `.gitignore` (edit) — add `go.work.sum` (regenerable local artifact per
  current workspace guidance; `go.work` itself stays committed and
  un-ignored).

### Edit: CI + build + dependabot

- `Makefile` (edit) — new `sdk-test` target
  (`cd sdk && GOWORK=off go test -race ./... -count=1`) + `sdk-test` added to
  `.PHONY` and to `verify` prerequisites.
- `.github/workflows/ci.yml` (edit) — `go` filter gains 4 lines (`sdk/go.mod`,
  `sdk/go.sum`, `go.work`, `go.work.sum`); new `changes` output `sdk` with its
  own filter block + `workflow_dispatch inputs.all` OR-clause; new `sdk-verify`
  job (`working-directory: sdk`, `GOWORK=off` steps); `cache-prime` keys
  extended to `hashFiles('go.sum', 'sdk/go.sum', 'go.work.sum')`; `result`
  (`CI passed`) `needs:` gains `sdk-verify`.
- `.github/dependabot.yml` (edit) — second `gomod` entry with
  `directory: /sdk`, same schedule/cooldown/labels/commit-message shape as the
  root entry.

### Edit: docs + skills + READMEs (mechanical path swaps + one new subsection)

`sdk/v1` → `sdk` in: `AGENTS.md` (Map row),
`contrib/skills/magelift-provider/SKILL.md` (Boundary),
`CONTRIBUTING.md`, `docs/adding-a-provider.md` (×2 incl. the rule table),
`docs/adr/0003-portable-contracts-vs-topology.md` (×2),
`docs/adr/0004-ports-and-adapters.md`, `docs/architecture.md` (×2),
`docs/versioning.md` (×2), `examples/custom-cli/README.md`, `tests/README.md`.
Plus the new SDK-pinning subsection in `docs/adding-a-provider.md`
(`require github.com/magelift/magelift/sdk vX.Y.Z`, tags `sdk/vX.Y.Z`, no
replace). Touched human pages go through humanizer, then remove-ai-marks.

### NOT touched (explicit)

- `.goreleaser.yaml` (stays exactly two builds; no `sdk` entry).
- `.release-please-manifest.json` (stays single `.` entry).
- `magelift-extend` skill (boundary wording uses `sdk.Module`, unaffected).
- No new ADR (pre-release import-path move breaks zero consumers).
- Branch protection (no new rules; required `CI passed` absorbs `sdk-verify`).
- Historical records (`intent/`, sealed evidence): never rewritten.
- No `tool` directives (repo pins tools via `go run ...@version`/actions;
  migrating is unrequested toolchain churn).
- `website/`, `scripts/`, `mkdocs.yml` (verified: zero `sdk/v1` references).

## Order of work

### 1. Move + rename + rewrite (root-module-local; green at every step)

- [x] 1.1 Record pre-change counts, capture the `go doc -all` export baseline,
  then `git mv sdk/v1/*.go sdk/` (37 files; no content edits in this step) —
  verify: `grep -rn "magelift/magelift/sdk/v1" --include='*.go' internal/ cli/ cmd/ examples/ | wc -l` recorded (expect 343) `&& ls sdk/*.go | wc -l` returns `37` `&& test ! -e sdk/v1` exits 0 `&& GOMAXPROCS=1 GOFLAGS=-p=1 go build ./sdk/` exits 0 (package `v1` still compiles at its new path).
- [x] 1.2 Rename the package clause `package v1` → `package sdk` in all 37
  moved files — verify: `grep -rh '^package ' sdk/*.go | sort -u` returns
  exactly `package sdk` `&& gofmt -l sdk/` returns empty.
- [x] 1.3 Rewrite the 332 `sdk "`-aliased import lines to the bare new path
  with `gofmt -r '"github.com/magelift/magelift/sdk/v1" -> "github.com/magelift/magelift/sdk"' -w` over gofmt-clean inputs, then drop the now-redundant
  `sdk ` alias prefix on exactly those lines (scripted line pass; no other
  lines touched) — verify: `grep -rn 'sdk "github.com/magelift/magelift/sdk"' --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty `&& gofmt -l` over the touched files returns empty.
- [x] 1.4 Rewrite the 11 qualifier files: import line → bare new path, then
  `v1.` → `sdk.` qualifiers (word-boundary, dot-required pattern; the 4
  `otlp/v1` URL literals cannot match) — verify: per-file token audit shows
  zero remaining `v1` qualifiers `&& grep -rn '"github.com/magelift/magelift/sdk/v1"' --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty.
- [x] 1.5 Update the 3 Go comment mentions (`sdk/v1` prose → `sdk`), then prove
  the whole tree compiles including tests — verify:
  `GOMAXPROCS=1 GOFLAGS=-p=1 go build ./...` exits 0 `&& GOMAXPROCS=1 GOFLAGS=-p=1 go vet ./sdk/... ./internal/cli/... ./internal/external/... ./internal/cloud/aws/... ./internal/cloud/gcp/... ./internal/cloud/scaleway/... ./cmd/...` exits 0 (compiles the 11 qualifier files' tests; full `./...` vet stays a CI job).
- [x] 1.6 Run the affected suites: SDK standalone plus every package owning a
  qualifier file — verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test` (serial,
  per-package, `-count=1`) exits 0 for `./sdk/`,
  `./internal/cli/`, `./internal/external/newrelic/`,
  `./internal/cloud/aws/observability/`, `./internal/cloud/aws/stack/`,
  `./internal/cloud/gcp/observability/`,
  `./internal/cloud/scaleway/observability/` (test packages compile via their
  parents; `cmd/` mains have no tests).
- [x] 1.7 Prove the public surface diff is package-name-only: normalized
  `go doc -all` parity plus moved-file diff shape — verify: `go doc -all
  ./sdk/ | sed 's/package sdk/package v1/;s|magelift/sdk|magelift/sdk/v1|g'`
  diffs empty against the 1.1 baseline dump `&& git diff -M <base> HEAD --
  sdk/ | grep -E '^[-+]' | grep -vE '^[-+]{3}' | grep -vE '^[-+]\s*package
  (v1|sdk)$'` returns empty (`<base>` = merge-base with `main`; rename
  detection shows package-clause-only deltas).

### 2. Module + workspace + require (the split)

- [x] 2.1 Create `sdk/go.mod` with `module github.com/magelift/magelift/sdk`,
  `go 1.27.0`, `toolchain go1.27.1`, no require block — verify: `head -5
  sdk/go.mod | grep -Fx 'module github.com/magelift/magelift/sdk' && grep -Fx
  'go 1.27.0' sdk/go.mod && grep -Fx 'toolchain go1.27.1' sdk/go.mod`.
- [x] 2.2 Run SDK tidy once; touch `sdk/go.sum` into existence (tidy creates
  no sums for zero deps); prove second tidy is a no-op — verify: `cd sdk &&
  GOWORK=off go mod tidy && test -f go.sum && GOWORK=off go mod tidy && git
  diff --exit-code -- go.mod go.sum` (serial env
  `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`).
- [x] 2.3 Prove the SDK module resolves standalone — verify: `cd sdk &&
  GOWORK=off go list -m github.com/magelift/magelift/sdk`.
- [x] 2.4 Create committed root `go.work` (`go 1.27.0`, `toolchain go1.27.1`,
  `use (` `.` + `./sdk` `)`); run `go work sync` (expect no `go.work.sum`
  with a zero-dep SDK); add `go.work.sum` to `.gitignore`; do NOT ignore
  `go.work` — verify: `grep -Fx 'go 1.27.0' go.work && grep -P -q '^\t\.$'
  go.work && grep -P -q '^\t\./sdk$' go.work && grep -Fx 'go.work.sum'
  .gitignore` (PCRE `-P`: plan authors note — BRE `\t` does not match tab).
- [x] 2.5 Prove root resolves the SDK through the workspace with default
  `GOWORK` — verify: `go list -m github.com/magelift/magelift/sdk && go build
  ./cli/... && go vet ./internal/platform/`.
- [x] 2.6 Assert the pre-tag steady state: NO root require yet (it cannot land
  until the proof tag exists — toolchain graph loading, verified empirically),
  and never a `replace` line — verify: `! grep -F
  'magelift/sdk' go.mod` exits 0 (no require, no replace) `&& ! grep -F
  'replace github.com/magelift/magelift/sdk' go.mod go.work sdk/go.mod` exits 0
  `&& GOMAXPROCS=1 GOFLAGS=-p=1 go build ./...` exits 0 (workspace resolves
  everything).
- [x] 2.7 Record root tidy blocked pre-tag (expected failure signature only)
  and prove `go.mod`/`go.sum` byte-identical across the attempt — verify: `go
  mod tidy 2>&1 | grep -F 'no matching versions for query "latest"'` exits 0
  (the SDK has no published version yet, so tidy cannot complete) `&& git diff
  --exit-code -- go.mod go.sum` exits 0 (failed tidy writes nothing; the
  require + successful tidy land in the 5.2 follow-up).
- [x] 2.8 Prove SDK dependency purity by construction plus standalone tests —
  verify: `cd sdk && for pat in '^github\.com/' '^golang\.org/' '^go\.'
  'magelift/internal/'; do GOWORK=off go list -deps . | grep -v
  '^github.com/magelift/magelift/sdk$' | grep -E "$pat" && exit 1; done`
  (no hits; the `grep -v` drops the module's own path, which `go list -deps`
  always prints first) `&& GOMAXPROCS=1 GOFLAGS=-p=1 GOWORK=off go test ./...
  -count=1`.

### 3. Makefile + CI + dependabot

- [x] 3.1 Add `Makefile` `sdk-test` target (`cd sdk && GOWORK=off go test
  -race ./... -count=1`), add `sdk-test` to `.PHONY` and to `verify`
  prerequisites; leave `extension-test`
  (`scripts/custom-extension-clean-room.sh`) byte-identical — verify: `make
  sdk-test` (serial under exported `GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB`,
  never parallel with `make test`).
- [x] 3.2 Extend the `ci.yml` `go` paths-filter with `sdk/go.mod`, `sdk/go.sum`,
  `go.work`, `go.work.sum` (exact quoted entries) — verify: `grep -n
  "sdk/go.mod" .github/workflows/ci.yml && grep -n "sdk/go.sum"
  .github/workflows/ci.yml && grep -n "'go.work'" .github/workflows/ci.yml &&
  grep -n "'go.work.sum'" .github/workflows/ci.yml`.
- [x] 3.3 Add `changes` output `sdk` with filter block (`sdk/**/*.go`,
  `sdk/go.mod`, `sdk/go.sum`, `go.work`, `go.work.sum`) plus the same
  `workflow_dispatch inputs.all` OR-clause as other outputs — verify: `grep -n
  "sdk:" .github/workflows/ci.yml && grep -n "sdk/\*\*/\*.go"
  .github/workflows/ci.yml`.
- [x] 3.4 Add `sdk-verify` job (gated on `needs.changes.outputs.sdk == 'true'`,
  NOT skipped for Dependabot): checkout (persist-credentials false),
  `setup-go` with `go-version-file: go.mod` (do NOT switch to `go.work`),
  cache restore keyed on `hashFiles('go.sum', 'sdk/go.sum', 'go.work.sum')`,
  then `working-directory: sdk` + `env: GOWORK: off` steps `go build ./...`,
  `go vet ./...`, `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1`,
  `go mod tidy && git diff --exit-code -- go.mod go.sum`, `govulncheck ./...`
  (via `golang/govulncheck-action` with `working-directory` if the pinned
  action supports it, else fallback pinned
  `go run golang.org/x/vuln/cmd/govulncheck@<hash> ./...`), plus a
  `golangci-lint` step from `sdk/` (config found via parent traversal of root
  `.golangci.yml`) — verify: `grep -n "sdk-verify"
  .github/workflows/ci.yml && grep -n "working-directory: sdk"
  .github/workflows/ci.yml && grep -n "GOWORK" .github/workflows/ci.yml`.
- [x] 3.5 Extend `cache-prime` cache keys from `hashFiles('go.sum')` to
  `hashFiles('go.sum', 'sdk/go.sum', 'go.work.sum')` (both the lookup block
  and the explicit-save block) and keep `go mod download` (workspace-aware,
  warms both modules) — verify: `grep -n "hashFiles('go.sum', 'sdk/go.sum',
  'go.work.sum')" .github/workflows/ci.yml`.
- [x] 3.6 Add `sdk-verify` to the `result` (`CI passed`) aggregator `needs:`
  array (without this the gate is advisory) — verify: `sed -n '/^  result:/,/^
  if: always/p' .github/workflows/ci.yml | grep -n 'sdk-verify'`.
- [x] 3.7 Validate workflow syntax after all `ci.yml` edits — verify: `go run
  github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml`.
- [x] 3.8 Add second `gomod` entry to `.github/dependabot.yml`
  (`directory: /sdk`, same `schedule: { interval: weekly }`, cooldown, labels,
  commit-message as root entry; mirror root groups if schema prefers) —
  verify: `grep -n -A3 'package-ecosystem: gomod' .github/dependabot.yml |
  grep -n '/sdk'`.

### 4. Docs + codegen + terminal gate

- [x] 4.1 Apply the mechanical `sdk/v1` → `sdk` path swaps in the 10 doc/skill
  files (AGENTS.md Map, provider SKILL Boundary, CONTRIBUTING.md,
  adding-a-provider ×2, ADR 0003 ×2, ADR 0004, architecture ×2, versioning ×2,
  custom-cli README, tests/README) — verify: `grep -rn 'sdk/v1' AGENTS.md
  CONTRIBUTING.md tests/README.md contrib/skills/magelift-provider/SKILL.md
  docs/adding-a-provider.md docs/adr/0003-portable-contracts-vs-topology.md
  docs/adr/0004-ports-and-adapters.md docs/architecture.md docs/versioning.md
  examples/custom-cli/README.md` returns empty.
- [x] 4.2 Add the SDK-pinning subsection to `docs/adding-a-provider.md` under
  `## Community binary` (adjacent to `### Extension build verification
  (RELEASE-03)`): `require github.com/magelift/magelift/sdk vX.Y.Z` snippet,
  explicit no-`replace` note, and the literal tag scheme `sdk/vX.Y.Z`; run
  humanizer then remove-ai-marks on this and every box-4.1 human page per repo
  rules — verify: `grep -n -F 'sdk/vX.Y.Z' docs/adding-a-provider.md && grep
  -n -F 'require github.com/magelift/magelift/sdk vX.Y.Z'
  docs/adding-a-provider.md`.
- [x] 4.3 (Optional, comments only) Add one-line SDK-tag-exclusion comments
  above `tags: ['v*']` in `release.yml:5` and `images.yml:9`; confirm trigger
  logic untouched and GoReleaser SDK-free — verify: `sed -n '3,6p'
  .github/workflows/release.yml | grep -F "tags: ['v*']" && sed -n '7,10p'
  .github/workflows/images.yml | grep -F "tags: ['v*']" && ! grep -n 'sdk'
  .goreleaser.yaml` (expect no `sdk` grep hit; `.goreleaser.yaml` itself
  unedited).
- [x] 4.4 Prove codegen resolves through the workspace unchanged (no logic
  edits) — verify: `go run ./cmd/genconfig --check && go run ./cmd/gendocs
  --check && go run ./cmd/gencertdocs --check`.
- [x] 4.5 Run the terminal local gate — verify: `make verify` exits 0.
- [x] 4.6 Final sweeps: stale import prefix gone, new count exact, URL/config
  pins intact — verify: `grep -rn "magelift/magelift/sdk/v1"
  --include='*.go' internal/ cli/ cmd/ examples/ sdk/` returns empty `&&
  grep -rn "magelift/magelift/sdk" --include='*.go' internal/ cli/ cmd/
  examples/ sdk/ | wc -l` equals the 1.1 count `&& go list -m
  github.com/magelift/magelift/sdk` succeeds `&& GOMAXPROCS=1 GOFLAGS=-p=1 go
  build -o /tmp/magelift-ext ./examples/custom-cli` succeeds.

### 5. Merge, then tag + proxy proof (USER RELEASE ACTION — not executed by the implementer)

Per AGENTS.md ask-first and the goal brief's no-tag rule, the implementer
stops after group 4. The user (or a release session) runs these ordered steps;
each lists its exact verify. The intent's proxy/tag scenarios stay open until
then and MUST be recorded in the change report as pending-user, never silently
dropped.

- [ ] 5.1 Merge the PR, documenting the `GOWORK=off` wart in the PR description
  (NOT in repo files): until the proof tag lands, `GOWORK=off` root builds
  fail because the required SDK version is not yet on the proxy; this
  disappears when `sdk/v1.0.0-rc.1` is pushed.
- [ ] 5.2 Push the lockstep proof tag from the merge commit (default
  `sdk/v1.0.0-rc.1`), then land the require in a follow-up commit on main
  (`go mod edit -require=github.com/magelift/magelift/sdk@v1.0.0-rc.1`, `go mod
  tidy` twice with the second a proven no-op, commit `go.mod`/`go.sum`) and
  confirm lockstep (identical `sdk/**.go` trees at both tags, root require
  matches, manifest still single-entry) — verify: `git tag sdk/v1.0.0-rc.1
  <merge-sha> && git push origin sdk/v1.0.0-rc.1` (allow proxy propagation;
  retry, do not force), then post-tag `go mod tidy && go mod tidy && git diff
  --exit-code -- go.mod go.sum` exits 0 `&& git diff v1.0.0-rc.1
  sdk/v1.0.0-rc.1 -- sdk/ -- ':!sdk/go.mod' ':!sdk/go.sum'` (empty) `&& go mod
  edit -json` shows the require at `v1.0.0-rc.1` `&& grep -c '"."' 
  .release-please-manifest.json` (expect 1).
- [ ] 5.3 Prove proxy resolution from a scratch module outside the repo (allow
  propagation delay; retry, do not force) — verify: `mkdir -p /tmp/sdk-proof
  && cd /tmp/sdk-proof && go mod init sdkproof 2>/dev/null; GOWORK=off
  GOPROXY=https://proxy.golang.org go list -m
  github.com/magelift/magelift/sdk@v1.0.0-rc.1`.
- [ ] 5.4 Prove `custom-extension-contract` builds `GOWORK=off` against the
  proxied SDK and stays `internal/`-free — verify: `GOWORK=off go build
  ./examples/custom-extension-contract && go list -deps
  ./examples/custom-extension-contract | (! rg 'magelift/internal/')`.
- [ ] 5.5 Prove SDK tags never trigger CLI/image releases: push `sdk/v9.9.9-test`,
  observe zero runs on `release.yml` and `images.yml`, then delete the tag
  immediately (push-then-delete hygiene) — verify: `git tag sdk/v9.9.9-test &&
  git push origin sdk/v9.9.9-test && gh run list --workflow release.yml --limit
  3` (no run for the tag) `&& gh run list --workflow images.yml --limit 3` (no
  run for the tag) `; git push --delete origin sdk/v9.9.9-test && git tag -d
  sdk/v9.9.9-test`.
- [ ] 5.6 (Optional, default do it) Create a notes-only GitHub Release for the
  proof tag for discoverability, no artifacts — verify: `gh release create
  sdk/v1.0.0-rc.1 --title 'SDK v1.0.0-rc.1 (lockstep)' --notes 'Lockstep with
  CLI v1.0.0-rc.1. Module: github.com/magelift/magelift/sdk. No binaries;
  consume via the Go module proxy.' && gh release view sdk/v1.0.0-rc.1 --json
  assets --jq '.assets | length'` (expect 0).

## Risks

- Rewrite miss in 343 lines: a skipped import or qualifier is a build error,
  not silent — mitigated by `gofmt -r` whole-tree passes, the stale-prefix
  sweeps (1.4, 4.6), per-file token audits for the 11 qualifier files, and
  `go build ./...` + affected suites (1.5, 1.6).
- Nested-module `./...` blindness: once split, root `go test -race ./...`
  (`Makefile test`), `golangci-lint run ./...` (`Makefile lint`), `go-licenses
  check ./...` (`Makefile license-check`), and CI `go-verify` (`go test`,
  `govulncheck`, `go-licenses`) plus CI `lint` (`args: ./...`) silently skip
  `sdk/` — mitigated by the load-bearing `sdk-verify` job + `make sdk-test`
  in `verify`; audit command at implementation time: `grep -rn 'go test\|go
  vet\|golangci\|govulncheck\|go-licenses' Makefile .github/ scripts/`.
- `GOWORK=off` wart window: between merge and the 5.2 require-commit,
  `GOWORK=off` root builds fail (no published SDK version to resolve: without
  the require there is nothing to resolve to, with it the tag is missing).
  Narrow by construction (minutes between merge, tag, and follow-up);
  document in the PR; never paper over with a temporary committed `replace`
  (in `go.mod` OR `go.work` — a workspace replace would be dev-only cruft
  with zero consumer benefit and a removal step to forget).
- Proxy propagation delay after tag push: `go list -m @version` may 404 for
  minutes — mitigated by retry-with-backoff in 5.3, never by re-tagging or
  force-pushing (published module versions are immutable; leave the proof tag
  in place even on rollback).
- `setup-go` `go-version-file` behavior: switching to `go.work` is unverified
  — mitigated by keeping `go-version-file: go.mod` in `sdk-verify` (SDK
  mirrors root toolchain `go1.27.1` exactly, so one toolchain serves both).
- `govulncheck-action` `working-directory` support: the pinned action may not
  accept it — fallback is pinned `go run golang.org/x/vuln/cmd/govulncheck@<hash>
  ./...` with `working-directory: sdk`; implementer picks whichever works and
  pins by hash.
- Dependabot second-entry syntax: a malformed second `gomod` entry breaks
  config validation — mitigated by mirroring the root entry's exact keys.
- Accidental `replace` commit: a local-debug `replace
  github.com/magelift/magelift/sdk => ./sdk` leaking into the PR breaks
  downstream consumers and masks skew — mitigated by the `! grep replace`
  assertion in 2.6 and a final pre-push `grep -rn '^replace' go.mod go.work
  sdk/go.mod` (expect no matches).
- Two-module tidy drift: root `./...` no longer covers `sdk/`, so forgetting
  the SDK tidy lets `sdk/go.sum` drift silently — mitigated by CI `sdk-verify`
  tidy no-op assertion and the two-command sequence (`go mod tidy` at root +
  `GOWORK=off go mod tidy` in `sdk/`).
- Doc-drift from path swaps: 15 mechanical lines across 10 human pages could
  mistarget prose — mitigated by exact-line edits (no blind replace-all across
  docs) plus humanizer + remove-ai-marks per touched page.

## Proof

End-to-end evidence for the whole spec (run serially under `GOMAXPROCS=1
GOFLAGS=-p=1 GOMEMLIMIT=1GiB`; never parallel heavy builds):

```bash
# 0. Pre-change records (before any edit)
grep -rn "magelift/magelift/sdk/v1" --include='*.go' internal/ cli/ cmd/ examples/ | wc -l  # 343
go doc -all ./sdk/v1/ > /tmp/sdk-doc-before.txt

# 1. Move + package rename
ls sdk/*.go | wc -l                     # 37
test ! -e sdk/v1 && echo MOVED-OK
grep -rh '^package ' sdk/*.go | sort -u  # package sdk

# 2. Rewrite complete, tree compiles, suites green
grep -rn "magelift/magelift/sdk/v1" --include='*.go' internal/ cli/ cmd/ examples/ sdk/  # empty
grep -rn "magelift/magelift/sdk" --include='*.go' internal/ cli/ cmd/ examples/ sdk/ | wc -l  # == 343
GOMAXPROCS=1 GOFLAGS=-p=1 go build ./...
go doc -all ./sdk/ | sed 's/package sdk/package v1/;s|magelift/sdk|magelift/sdk/v1|g' | diff /tmp/sdk-doc-before.txt -  # empty

# 3. Module files exist and resolve standalone (GOWORK=off)
head -1 sdk/go.mod                          # module github.com/magelift/magelift/sdk
grep -Fx 'go 1.27.0' sdk/go.mod && grep -Fx 'toolchain go1.27.1' sdk/go.mod
test -f sdk/go.sum
(cd sdk && GOWORK=off go list -m github.com/magelift/magelift/sdk)

# 4. Workspace content + workspace resolution
grep -Fx 'go 1.27.0' go.work && grep -P -q '^\t\.$' go.work && grep -P -q '^\t\./sdk$' go.work
go list -m github.com/magelift/magelift/sdk && go build ./cli/... && go vet ./internal/platform/

# 5. Pre-tag steady state: no require, no replace (require lands post-tag in 5.2)
! grep -F 'magelift/sdk' go.mod
! grep -F 'replace github.com/magelift/magelift/sdk' go.mod go.work sdk/go.mod
GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext ./examples/custom-cli

# 6. Purity by construction + tidy no-op + SDK tests standalone (self-path filtered: list always prints own module first)
for pat in '^github\.com/' '^golang\.org/' '^go\.' 'magelift/internal/'; do (cd sdk && GOWORK=off go list -deps . | grep -v '^github.com/magelift/magelift/sdk$' | grep -E "$pat") && echo "PURITY-HIT:$pat"; done  # expect no hits
(cd sdk && GOWORK=off go mod tidy && git diff --exit-code -- go.mod go.sum)
(cd sdk && GOMAXPROCS=1 GOFLAGS=-p=1 GOWORK=off go test ./... -count=1)

# 7. Tag scheme documented; GoReleaser untouched; lockstep files
grep -F -q 'sdk/vX.Y.Z' docs/adding-a-provider.md
grep -c 'id: magelift' .goreleaser.yaml        # exactly the two known builds
! grep -n 'sdk' .goreleaser.yaml
cat .release-please-manifest.json  # single "." entry

# 8. CI wiring present
grep -q 'sdk/go.mod' .github/workflows/ci.yml && grep -q 'sdk/go.sum' .github/workflows/ci.yml
grep -q "'go.work'" .github/workflows/ci.yml && grep -q "'go.work.sum'" .github/workflows/ci.yml
grep -q 'sdk-verify' .github/workflows/ci.yml
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml

# 9. Codegen trio green through the workspace
go run ./cmd/genconfig --check && go run ./cmd/gendocs --check && go run ./cmd/gencertdocs --check

# 10. Terminal local gate
make verify

# 11. Proxy pinning from a scratch module (post proof-tag push, USER ACTION)
mkdir -p /tmp/sdk-proof && cd /tmp/sdk-proof && (go mod init sdkproof 2>/dev/null || true)
GOWORK=off GOPROXY=https://proxy.golang.org go list -m github.com/magelift/magelift/sdk@v1.0.0-rc.1
cd /home/dev/workspace/magelift
GOWORK=off go build ./examples/custom-extension-contract
go list -deps ./examples/custom-extension-contract | (! rg 'magelift/internal/')
```
