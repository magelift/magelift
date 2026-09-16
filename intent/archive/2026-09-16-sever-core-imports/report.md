---
slug: sever-core-imports
verified: 2026-09-16
verdict: pass
---

# Report: sever core imports

## What shipped

Non-test code under `cli/` and `internal/cli/` imports zero
`internal/cloud/*` packages (was 5 + 3 + 1 direct imports). Provider hooks now
resolve through explicit registration:

- `internal/cli/root.go`: `Hooks` gains `NewCleanupProvider` and
  `MediaEndpoint`; `options` gains `mediaEndpoint` (`newCleanupProvider` field
  already existed); `newCommandWithHooks` threads all six (nil stays nil).
- `internal/cli/cleanup.go`: `defaultCleanupProvider` is now an `options`
  method with dispatch body only (prefer `o.newCleanupProvider`, then
  `o.newGCPCleanupProvider` for `gcp` ledgers, else
  `cleanup.UnsupportedProvider`); `cleanupProviderFor` delegates to it; the
  GCP/OVH/Scaleway construction bodies moved to the aggregator; 3 cloud
  imports plus now-unused `scw`/`credentials` imports dropped (`strings`/`time`
  stay — used elsewhere).
- `internal/cli/env.go`: `newMediaS3Client` is now an `options` method reading
  `o.mediaEndpoint` (nil yields `""`, hook errors keep the `invalid(err)`
  wrap, rest verbatim); call site updated; `awsendpoint` import dropped (AWS
  SDK imports stay — not `internal/cloud`).
- `internal/registry/hooks.go` (new): `RegisterHooks() internalcli.Hooks`
  owns all provider hook construction with verbatim-moved bodies — AWS/GCP
  composer secrets (including the `GetParameter` refusal), the leftover
  backup destroyer, both GCP cleanup constructors (kept distinct: native-API
  vs `NewCloudSQLAPI` variants), the OVH/Scaleway/gcp-fallback cleanup switch,
  and `MediaEndpoint` (`awsendpoint.FromEnv`). No `init`, no globals, no
  constructor runs at registration.
- `cli/cli.go`: new sibling `NewWithExtensionsAndHooks(hooks, extensions...)`;
  `NewWithExtensions` keeps its exact signature and delegates with
  `registry.RegisterHooks()`; `firstPartyHooks`, both glue types, 5 cloud
  imports, and now-unused `context`/`strings`/`cleanup`/`platform` imports
  deleted.
- `cmd/magelift/main.go`: explicit registration site —
  `cli.NewWithExtensionsAndHooks(registry.RegisterHooks())`; error handling,
  `Execute`, `ExitCode` identical.
- Tests: new `internal/registry/hooks_test.go` (6 tests: all-hooks non-nil,
  credential-free validation paths, media env default + loopback refusal, 3
  relocated cleanup-construction tests); `internal/cli/root_test.go` gains the
  threading test (zero-hooks fallback, set-hook dispatch, media default +
  override); the 3 relocated tests deleted from `cleanup_test.go` (coverage
  moved with the code, 1:1).
- Docs: `docs/adding-a-provider.md` step 8 names the `RegisterHooks` seam and
  extends the Pulumi wording with the zero-`internal/cloud/*` rule.

## Deviations from plan

1. Box 3.2 required test edits the plan mispredicted as "no edit expected":
   `cleanup_test.go` called the old free `defaultCleanupProvider` at 3 sites,
   so the package could not compile without touching it. The 3 tests moved
   verbatim (assertions identical, only the called seam changed) to
   `hooks_test.go`, renamed to the new seam. Net coverage unchanged
   (cli 289→286→287 with the new subtest; registry 0→3→6).
2. Box 3.5's literal 3-line form (`modules, err := registry.NewDefault()`
   then the sibling) is uncompilable — the sibling owns single registration
   and takes no modules param, leaving `modules` unused (plus double
   `NewDefault` work). Main calls the sibling with explicit
   `registry.RegisterHooks()` instead; the explicitness requirement the box
   verifies (RegisterHooks grep on `cmd/magelift/`) is fully met.
3. Box 1.2's set-hook MediaEndpoint dispatch assertion could not pass at 1.2:
   `o.mediaEndpoint` is write-only until the 3.3 method conversion (nothing
   reads it), so set-hook dispatch is unobservable before 3.3. Landed 1.2 with
   cleanup dispatch (set + zero) fully behavioral, media default-behavior, and
   the MediaEndpoint Hooks-literal compile proof; extended the same test
   function at 3.3 with the set-hook override assertion. Same file, no new
   test files — the plan's "only root_test.go gains a test" holds.
4. Box 4.1 `make verify` fails at its first gate on the known pre-existing
   sealed-evidence digest mismatch (same file as the goal brief; fails
   identically at clean HEAD). Ran every remaining component individually:
   cli-docs-check, fmt-check, clean-room, docs, workflow-check, lint
   (0 issues), license-check all exit 0; phpunit shows the same 3 pre-existing
   environmental failures as order 15 (custom-PHP `LD_LIBRARY_PATH` strip,
   `build/` untouched); full `go test -race ./...` skipped per brief.
5. Spec's `grep -c 'Module{'` → 6 scenario is defective: it also matches the
   `[]platform.StackModule{` slice literal, so it returns 7 at HEAD and after
   any correct implementation. Substance verified instead: exactly the six
   spec-named entries (`awsops`, `awseksops`, `gcpops` ×2, `ovhstack`,
   `scwstack`), `NewDefault` body untouched (registry.go diff is one comment
   line), registry suite green.
6. `remove-ai-marks` service still unreachable (curl exit 7); no local cleaning
   attempted. Humanizer review of the step-8 edit: plain declarative, no tells,
   no edits needed. Page scans clean; box greps re-verified.
7. Incidental: three `go build` binaries landed at the repo root during gate
   runs (`custom-cli`, `gendocs`, `magelift`); deleted, untracked, never part
   of the diff.

## Verification

### Completeness

All 12 plan boxes executed: 1.1, 1.2, 2.1–2.3, 3.1–3.6, 4.1. Every `###
Requirement:` in `spec.md` has direct evidence:

- `cli/` zero cloud imports: quoted-path grep empty; alias grep
  (`awssecrets|gcpsecrets|gcpresilience|gcpstack|gcp/naming`) empty; `go build
  ./cli/` exit 0.
- `internal/cli/` zero cloud imports: quoted-path grep empty (the
  `sign.go:154` comment mention survives by design — greps use the quoted
  import path); `go list -f '{{ join .Imports "\n" }}'` grep empty.
- Explicit registration: `RegisterHooks` grep on `cmd/magelift/` non-empty;
  `func init(` grep over `cli/`, `internal/cli/`, `internal/registry/`
  (non-test) empty.
- Behavior preserved: `internal/cli` 287 green, `internal/registry` 6 green,
  `go build ./examples/custom-cli/` exit 0 (signature grep exact),
  `go build ./cmd/magelift/` exit 0; `make verify` per deviation 4.
- Gendocs linkable: Pulumi-SDK grep in non-test `internal/cli/` empty;
  `go build ./cmd/gendocs/` exit 0.
- Registry six modules: per deviation 5 (six named entries, body untouched,
  `func NewDefault` present).
- SDK/config locks: quoted-path greps over `sdk/v1/` and `internal/config/`
  (non-test) empty.
- Error-string preservation: all ten verbatim strings (both GCP project
  variants with their distinct `errors.New`/`fmt.Errorf` wrappers, OVH +
  Scaleway validations, all four Scaleway profile errors, both leftover
  messages, both client-creation wraps, adapter refusal) grep in `hooks.go`;
  `secretRegion` untouched in `internal/cli/build.go` (composer-credential
  cases green in the 287).
- No-cycle: `go list -deps ./internal/cli/` contains no `internal/registry`;
  only outer `cli/` imports it. Registry suite compiling proves the new
  `registry → internal/cli` edge is acyclic.

### Correctness

Bar is the intent's proposed outcome: zero `internal/cloud/*` imports in
non-test `cli/` + `internal/cli/`, hooks resolved through explicit
registration, behavior unchanged, gates green with a smaller link surface.
All met: both packages severed (grep- and `go list`-proven, including the
alias and Pulumi sweeps); `cmd/magelift` is the explicit site with no init
magic; the 287-test CLI suite (including secret-region, composer-credential,
and cleanup tests), the 6-test registry suite, and the custom-cli, gendocs,
and full-CLI links are green with bodies moved verbatim and error strings
identical. The link surface shrinks as designed: `internal/cli` no longer
reaches any cloud SDK; only `registry` (which `gendocs` never imports)
constructs provider clients. Not a UI change; observable moments are the
empty import greps, the green suites, and the successful links.

### Coherence

Diff follows the spec Design: `internal/cli` keeps the port shapes and
dispatch/fallback logic; `internal/registry` is the single core-adjacent
aggregator importing the 8 provider hook packages; `NewWithExtensions`
signature stable with the hooks-first sibling (variadic extensions stay last);
`internal/cleanup` provider structs untouched; `platform.ModuleRegistry` and
`infra.RegisterTarget` semantics unchanged; single `go.mod`, no tags, no
subprocess changes. `docs/adding-a-provider.md` step 8 updated in the same
change per the carried-forward default.

## Findings

- WARNING — `make verify` red on pre-existing sealed-evidence digest mismatch
  (same file, same line as orders 15–16). Unchanged by this intent; owned by
  the certification/seal track. `Makefile:281`
- WARNING — `remove-ai-marks` HTTP service down three intents running
  (connection refused). Hand humanizer review + scans only. Environmental.
- SUGGESTION — spec's `Module{` count scenario matches its own slice literal;
  future specs should anchor entry counts (e.g. `ops.Module{|stack.Module{`).
  `intent/sever-core-imports/spec.md:98`
- SUGGESTION — plan's "no edit expected" test predictions missed 3 direct
  callers (`cleanup_test.go` → moved free function); predictions about test
  seams should be verified with a caller grep at plan time.
  `intent/sever-core-imports/plan.md:31`

## Not checked

- Full `go test -race ./...`: skipped per goal brief. Compensated with both
  affected suites (293 tests), plus `go build` of `./cli/`, `./cmd/magelift/`,
  `./cmd/gendocs/`, `./examples/custom-cli/`.
- Live CLI runs: no-cloud-spend per brief. Behavior equivalence rests on
  verbatim-moved bodies, identical error strings, and green suites.
- Verified in implementing session (no forked verifier; re-running the links
  plus suites elsewhere duplicates identical evidence).

## Verdict

Pass. Core severed from provider imports with explicit registration, behavior
preserved verbatim, all green gates green. The red `make verify` leg and the
two spec/plan-check defects are pre-existing or cosmetic, documented with
proof, and change nothing about the outcome.
