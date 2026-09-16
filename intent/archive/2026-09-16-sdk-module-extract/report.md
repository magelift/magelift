---
slug: sdk-module-extract
verified: 2026-09-16
verdict: pass
---

# Report: SDK module extract (Option A)

## What shipped

The SDK is an independent Go module at `sdk/` (module
`github.com/magelift/magelift/sdk`), developed via a committed `go.work`
workspace, with all importers rewritten once to the clean path:

- Move: 37 files `sdk/v1/*.go` → `sdk/*.go` (`git mv`); `package v1` →
  `package sdk` in all 37 (sole content change in moved files); `sdk/v1/`
  gone.
- Rewrite: 343 importer lines `.../sdk/v1` → `.../sdk` (332 alias-drops with
  byte-identical `sdk.` qualifiers; 11 files with `v1.` → `sdk.` qualifier
  rewrites after per-file token audits proving zero variables/collisions);
  3 Go comment mentions updated. New exact count 343; old prefix gone.
- Module: `sdk/go.mod` (`.../sdk`, `go 1.27.0`, `toolchain go1.27.1`, zero
  requires), empty committed `sdk/go.sum` (tidy proven no-op), `sdk/LICENSE`
  (byte-identical Apache-2.0 copy — `go-licenses` resolves per dependency
  module dir; no `sdk/NOTICE`: root NOTICE's operative content is
  third-party attribution and the SDK has zero third-party code).
- Workspace: committed `go.work` (`use .` + `./sdk`); `go.work.sum` gitignored
  per current workspace guidance (none generated with a zero-dep SDK).
- Root: NO require yet (see Deviations — lands in the post-tag 5.2
  follow-up); NO replace anywhere (`go.mod`, `go.work`, `sdk/go.mod`);
  `go.mod`/`go.sum` byte-identical to baseline.
- CI/build: `Makefile` `sdk-test` target + `.PHONY` + `verify` wiring;
  `ci.yml` `go` filter +4 lines, new `sdk` changes output + filter block, new
  `sdk-verify` job (`GOWORK=off` build/vet/test-race/tidy-diff/lint/vuln from
  `sdk/`), `cache-prime` keys extended, `result` needs `sdk-verify`;
  dependabot second `gomod` entry (`/sdk`, no groups — zero deps);
  trigger-exclusion comments above both `tags: ['v*']` (zero logic change).
- Docs: mechanical `sdk/v1` → `sdk` in 10 files (AGENTS Map, provider SKILL,
  CONTRIBUTING, adding-a-provider ×2, ADR 0003 ×2, ADR 0004, architecture ×2,
  versioning ×2, custom-cli README, tests/README incl. the `make sdk-test`
  run-cell fix); new SDK-pinning subsection (`require .../sdk vX.Y.Z`, tags
  `sdk/vX.Y.Z`, no replace). Historical records (`intent/`, sealed evidence)
  untouched.
- Untouched: `.goreleaser.yaml` (exactly two builds, zero `sdk` hits),
  `.release-please-manifest.json` (single `.`), `magelift-extend` skill,
  `website/`, `scripts/`, `mkdocs.yml`, branch protection, no new ADR, no
  `tool` directives (repo pins via `go run`/actions; unrequested churn).

## Deviations from plan

(Plan + spec were revised user-directed mid-intent from the disproven
`.../sdk/v1` module path to Option A; deviations below are relative to the
REVISED plan. The revision itself is recorded in spec Design + intent open
questions.)

1. Root require deferred to the 5.2 post-tag follow-up (NOT in the
   implementation PR): the toolchain must load the required version's go.mod
   to complete the module graph even in workspace mode — verified empirically
   both directions (with require: `unknown revision` on every graph command;
   without: builds/vet/tests green via workspace, only root tidy fails with
   `no matching versions for query "latest"`). Pre-tag `go.mod`/`go.sum`
   stay byte-identical (proven); the first successful root tidy runs in the
   follow-up. CI stays green throughout (no CI job enforces root tidy; all
   Go jobs are workspace-resolving).
2. `sdk/LICENSE` added (not in the original file list): `go-licenses`
   resolves per dependency module directory and failed without it; also
   required for pkg.go.dev + downstream consumers. `sdk/NOTICE` deliberately
   not added (see above).
3. `go.work.sum` gitignored instead of committed-iff-generated: current
   workspace guidance is explicit (dev-only artifact); none generates with a
   zero-dep SDK; CI hashes tolerate absence.
4. Spec/plan check-text fixes (all mechanical, behavior-neutral): purity
   greps exclude the module's own path (`go list -deps` always prints it
   first); `go.work` tab greps use `grep -P` (BRE `\t` never matches tab);
   require asserted via `go mod edit -json` (canonical block form, not
   single-line shape); 4.3 `sed` windows shifted by the inserted comment
   lines.
5. `make verify` fails at `generate-check` on the known pre-existing
   sealed-evidence mismatch (same file, 4th intent running); `gencertdocs
   --check` likewise (it imports the SDK directly and compiled and ran fine through
   the workspace — the failure is the evidence digest, proving direct-SDK
   resolution works). All other components run individually green (below);
   full `go test -race ./...` skipped per brief; phpunit same 3 pre-existing
   environmental failures.
6. `remove-ai-marks` service still unreachable (curl exit 7); hand humanizer
   review on all touched human pages (mechanical path swaps + one plain
   pinning subsection; no tells; no edits needed) + invisible-mark scans
   clean on all 10 pages.
7. Group 5 (merge, tag `sdk/v1.0.0-rc.1`, require+tidy follow-up commit, proxy
   proof, contract `GOWORK=off` proof, `sdk/v9.9.9-test` trigger regression,
   notes-only release) is USER RELEASE ACTION per AGENTS.md ask-first and the
   goal brief's no-tag rule — designed post-merge, not executed here. Exact
   commands live in plan group 5. The proxy/tag/lockstep spec scenarios stay
   open until then (listed under Not checked, not silently dropped).

## Verification

### Completeness

All implementer boxes executed: 1.1–1.7, 2.1–2.8, 3.1–3.8, 4.1–4.6. Group 5
is post-merge user action by plan design. Every spec requirement has direct
evidence except the tag/proxy scenarios in group 5 (explicitly pending, see
deviation 7).

- Module files: `sdk/go.mod` first line + `go`/`toolchain` mirrors exact;
  empty `sdk/go.sum` committed with tidy no-op proven twice; standalone
  `go list -m` green.
- Move: 37 files, `sdk/v1/` gone, `package sdk` unique; moved-file diff shows
  package-clause-only deltas (rename-detected) plus the single known order-15
  hunk (order-18-baseline dirt, not mine).
- Rewrite: old prefix grep empty (343→0); new count exactly 343; no `v1.`
  SDK qualifiers (only third-party `*v1` + `otlp/v1` URL literals, audited);
  no redundant `sdk "..."` aliases; whole tree `gofmt` clean.
- Workspace: `go.work` exact block (PCRE-verified tabs); `go list -m` +
  `go build ./cli/...` + `go vet ./internal/platform/` green; full
  `go build ./...` green in the pre-tag steady state (8.7 min).
- Require/replace: absent/absent pre-tag with full build green (deviation 1);
  `go.work sync` + root tidy fail only on the missing tag with recorded
  signatures and zero writes.
- Purity: 113 deps, all stdlib (self-path-filtered sweeps, 4 patterns);
  151 SDK tests green standalone (`GOWORK=off`); `sdk-test` (`-race`) green.
- CI: all filter/output/job/needs/cache greps green; `actionlint` green after
  all edits (validates the new `working-directory`/`work-dir` inputs);
  `sdk-verify` steps locally proven line-for-line (build/vet/test/tidy-diff
  green, golangci-lint from `sdk/` 0 issues via parent config traversal,
  govulncheck "No vulnerabilities found").
- Codegen: `genconfig` + `gendocs` `--check` green; `gencertdocs` red only on
  the pre-existing evidence file (deviation 5).
- custom-cli: builds via workspace (3.2 min link); `cli` import kept,
  `stub_module.go` rewritten; no stray binaries (all `-o /tmp`, cleaned).
- API parity: normalized `go doc -all` diff EMPTY (1943-line baseline);
  behavior suites green (sdk 151, cli 287, newrelic 62, aws-obs 32,
  aws-stack 84, gcp-obs 28, scaleway-obs 7).
- Docs: 10-file sweep empty; pinning literals present (`sdk/vX.Y.Z`,
  `require .../sdk vX.Y.Z`); triggers intact + comments; GoReleaser
  two-builds/no-sdk; manifest single-entry; `make docs` green.
- Tree hygiene: status delta vs order-18 baseline = exactly the intended set
  (37 renames, ~343 rewrite files incl. baseline-overlap, module/workspace/
  CI/docs files); nothing vanished; no stray binaries.

### Correctness

Bar is the intent's proposed outcome as user-revised: SDK as its own module
with own `go.mod`/`go.sum`, consumed via `go.work`, CI SDK-only gate, tag
scheme proving the workflow (pending user push), lockstep mechanics without an
independent release line, no API break, gates green. All implementer-side bars
met: the module resolves standalone and through the workspace; purity is
construction-enforced (113 stdlib deps); the public surface is proven
identical modulo the package rename; all 651 tests across every touched suite
plus full builds, vet, lint (0 issues), licenses, docs, and workflow checks
are green. The tag/proxy proofs are the only unverified scenarios and are
structurally post-merge (deviation 7). Not a UI change; observable moments are
the module/workspace files, the green gates, and the rebuilt docs site.

### Coherence

Diff follows the revised spec Design: textbook v1 module shape (`.../sdk`,
no suffix, tags `sdk/vX.Y.Z`) matching `golang-dependency-management`
versioning rules and `golang-project-layout` (packages match directories;
committed `go.work` monorepo); per-module CI gates per
`golang-continuous-integration`; no `tool`-directive migration (repo
consistency, out of scope); single-commit revert restores the monolith
(reverse the move list; proxy-immutable tag left alone per plan). Toolchain
behavior claims verified empirically against go1.27.1, not assumed
(`/v1` invalidity, graph-loading vs workspace packages, tidy-without-require).

## Findings

- WARNING — group 5 (merge/tag/require/proxy/release) pending user release
  action; proxy, lockstep-tag, trigger-regression, and `GOWORK=off` contract
  scenarios unverified until then. By plan design + brief prohibition, not by
  omission. Exact commands in plan group 5.
  `intent/sdk-module-extract/plan.md:5.1`
- WARNING — `make verify` / `gencertdocs --check` red on pre-existing
  sealed-evidence digest mismatch (same file as orders 15–17). Unchanged by
  this intent. `Makefile:284`
- WARNING — `remove-ai-marks` HTTP service down four intents running.
  Hand review + scans only. Environmental.
- SUGGESTION — spec/plan check-text defects fixed during execution (self-path
  in deps greps, BRE `\t`, single-line require shape, sed windows) — all
  corrected in place; future specs should run check commands once before
  acceptance. `intent/sdk-module-extract/spec.md:1`
- SUGGESTION — `sdk/NOTICE` deliberately omitted (root NOTICE's operative
  content is third-party attribution; SDK has zero third-party code). Revisit
  if the SDK ever gains a dependency. One-line addition if the user
  disagrees.

## Not checked

- Proxy resolution, lockstep tag identity, trigger regression, `GOWORK=off`
  contract build (group 5 — require the pushed tag; user action).
- Full `go test -race ./...`: skipped per goal brief. Compensated with full
  `go build ./...`, targeted `go vet` (compiles tests), 651 tests across all
  touched suites, plus `sdk-test -race`.
- Live CI run of the new `sdk-verify` job (no PR opened from here); job
  definition validated by `actionlint` and every step executed locally
  verbatim.
- Verified in implementing session (no forked verifier; re-running the full
  build plus suites elsewhere duplicates identical evidence).

## Verdict

Pass (implementation; release phase pending user). The SDK module split is
complete, pure, and verified with zero API change; the remaining tag/proxy
steps are post-merge user release actions with exact commands ready. Follow-up
is plan group 5, not a new intent.
