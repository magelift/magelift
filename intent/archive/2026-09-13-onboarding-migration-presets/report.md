---
slug: onboarding-migration-presets
verified: 2026-09-13
verdict: pass
---

# Report: install, import or preset, deploy

## What shipped

- `magelift init --provider aws|gcp` (default `aws`) writes a starter
  with preview, staging, and production env blocks. Both starters
  state search `disabled` explicitly (the certified cell) and pass
  `config validate` with no other edits. Unknown providers fail
  before the overwrite check with MageLift named as authority.
  Satisfies provider starters plus authority naming.
- The `<stem>.unmapped.md` sidecar keeps its key table and gains a
  "What to do" checklist: one action per family (hooks, crons,
  relationships, runtime, dependencies, stage, other), a generic
  ACC-before/MageLift-after snippet for hooks using the real
  `build.hooks` shape, and doc links. No PaaS values echo (the key
  type has no value field). Satisfies the checklist scenario.
- `doctor` sets `Next` from the first non-ok check: config families
  point at `magelift config validate`, missing dependencies at
  `magelift doctor --install-dependencies`, unknown IDs default to
  validate. Healthy reports keep the bootstrap Next (now
  `--env preview`, the first sorted env). Satisfies doctor-names-fix.
- Getting-started step 3 leads with `init --provider`, states both
  starter shapes (envs, queue defaults, explicit disabled search),
  and points at `config effective` plus the sample shop. User skills
  needed no edits (migrate already centers the sidecar; configure
  contradicts nothing).
- Existing tests that did string surgery on the old single-env
  starter were updated to the new shape (ci, preview, root,
  releases, dev suites).

## Deviations from plan

None. One scope addition inside the intent: starters state search
`disabled` explicitly rather than inheriting the preset managed
default, matching the certified cell and the configure skill's
disposable-preview preference.

## Verification

### Completeness

All 5 plan boxes ticked. All 4 spec requirements evidenced; all 7
scenarios observed.

### Correctness

- Starters: new `TestInitProviderStartersValidate` (aws, gcp),
  `TestInitProviderDefaultsToAWS`, `TestInitUnknownProviderRefusedBeforeOverwrite`
  green. Live drive: built CLI, `init --provider` plus `config
  validate` exit 0 for both; `config effective --env preview`
  shows `queueMode: db`/`searchMode: disabled` (AWS) and
  `queueMode: database`/`openSearchMode: disabled` (GCP).
- Sidecar: new `unmapped_test.go` pins all seven families, the
  hook before/after, and absence of the fixture's secret-marked
  values. Live drive on the ACC unmapped fixture: exit 2, sidecar
  renders table plus checklist, grep confirms no value bytes.
- Doctor: new Next tests (build failure, missing pulumi, env
  failure) green; live `doctor -o json` on the AWS starter shows
  `status: ok`, `next: magelift bootstrap --env preview`.
- Docs: `make docs` strict green after the getting-started edit.
- Suites: `internal/cli` 275 pass, `internal/paasimport` 25 pass,
  plus `internal/config`, resepectively 490 combined with `-race`;
  `go build ./...` and `gofmt` clean.
- Human-observable moments: the init/validate/doctor/effective
  transcript above plus the rendered sidecar sample, all driven
  through the built binary.

### Coherence

Flags follow the existing init flag style; starters mirror the
sample-shop env inheritance; sidecar guidance mirrors the migration
doc's side-by-side and the frozen D-07 allowlist; the doctor Next
map defaults safely for future check IDs. No config schema changes.

## Findings

- SUGGESTION — the AWS starter's staging/production envs emit
  "Aurora MySQL is experimental" warnings on validate (preset
  database default, pre-existing). Consider an `rds-mysql` starter
  note or preset review in a later intent. Observed, not introduced
  here.
- SUGGESTION — full-repo `go test -race ./...` exhausts the 7.9 GB
  `/tmp` tmpfs at link time in this environment (all failures were
  "no space left on device", none compile or test failures).
  Re-ran affected packages with disk-backed `TMPDIR`: green. A
  repo note or Makefile `TMPDIR` guard would save the next runner.

## Not checked

- Verified in implementing session.
- A real ACC repo import end to end (fixture-driven only); top
  doctor failures from real trials (unknown until trials run, per
  the spec's carried question).
- Full-repo race suite in one invocation (environmental tmpfs
  limit; affected packages plus full build verified instead).

## Verdict

Pass. A new team gets a validating three-env starter per certified
provider, a checklist sidecar, and a doctor that names the fix.
