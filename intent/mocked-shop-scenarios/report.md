---
slug: mocked-shop-scenarios
verified: 2026-09-16
verdict: pass
---

# Report: synthetic scenario foundation (mocks prove contracts, not stores)

## What shipped

Honest offline coverage with original fixtures, no store claims:

- 15 original fixtures under `tests/fixtures/synthetic/`: GCP + AWS
  preview configs (both load, validate, resolve, and registry-plan clean
  offline), four single-failure invalid fixtures (unknown provider,
  unsupported version, missing required field, expired preview), and
  synthetic ACC/Upsun importer inputs with deliberate mapped plus unmapped
  surface — all example-only identities, zero secrets.
- `tests/synthetic/suite_test.go` (`synthetic` tag, 9 tests): registry
  routing to real module Plans, fail-closed unknown-target dispatch, public
  contract bridge (valid fake registers and plans; zero-target descriptor
  and mismatched plan rejected), classified invalid-fixture failures,
  expired-preview plan refusal, importer mapping with unmapped-subset
  asserts plus version-error paths (malformed type at Map, php:7.4 at
  Resolve), localdev plan plus compose asserts with no containers,
  credential-denylist refusal via TestMain, and the gaps table with the
  file-back mechanism.
- `tests/synthetic/hygiene_test.go`: example-only identities, RFC1918-only
  IPv4, example account ID, and no secret-like assignments — with inline
  pass/fail self-test samples. Full suite: 11/11 green with creds stripped,
  nonzero exit with any denylisted variable set (all six families proven).
- `tests/README.md` suite row plus the fixture README stating scope,
  originality, and the no-store-proof boundary.

## Deviations from plan

1. The HISTORICAL `spec.md`/`plan.md` (superseded 2026-09-16 scope) were
   overwritten with the rewritten-scope documents. The superseded versions
   remain in git history (commit `612c98a`); SDLC operates on current
   `spec.md`/`plan.md`, and duplicating history into extra filenames would
   confuse the gates.
2. Importer version cases run as `t.TempDir()` variants derived from the
   committed ACC input instead of committed 9th/10th input dirs — same
   coverage, minimal committed tree.
3. Fixture YAML/dotfiles went through marks `/inspect` (all 0 suspicious),
   not `/clean`: the service correctly requires a Layer B rewrite backend
   for plain text, and rewriting data files would corrupt them. READMEs went
   through full `/clean` unchanged.
4. The AWS valid fixture carries full stack-spec fields (cert ARNs, KMS,
   secrets, DB identity, VPC CIDR, catalog) discovered empirically so both
   valid fixtures prove full offline planning rather than dispatch-only.

## Verification

### Completeness

All 9 plan boxes ticked (1.1–1.4, 2.1–2.3, 3.1, 3.2), each after its verify
clause passed. Every `### Requirement:` in `spec.md` has direct evidence:

- Fixtures: 15 files exist as listed; hygiene gate greens the tree;
  invalid fixtures fail with `target.provider must be…`,
  `must be an exact Magento release`, `target.gcp.project is required`,
  and plan-time `expired` respectively.
- Offline layer: full suite exits 0 creds-stripped (11/11); exits nonzero
  with each denylisted family set (proven for AWS/GCP/OVH/Scaleway/Pulumi/
  harness markers); routing asserts module selection; contract asserts
  bridge accept/reject; importer asserts exit-0 mapping with unmapped
  subsets plus both version-error paths.
- Local layer: `TestLocalPlanning` asserts the nginx image, mailpit shaping,
  and compose rendering with zero container operations (suite has no Docker
  dependency).
- Gaps: table non-empty with owned live-only entries; `ownerFor` unit-tested
  for known and unknown capabilities; mock inventory documented in the
  package comment; no suite or README line claims store-level proof
  (reviewed: verdicts scope to routing, contracts, failures, validation,
  offline planning).

### Correctness

Bar is the intent's proposed outcome: three honest layers with original
fixtures, loud gaps, and no store proof from mocks. All met: layer 1 runs
real config/registry/importer code with fakes only at the module boundary;
layer 2 runs real localdev planning with asserts on rendered output; layer 3
is owned elsewhere by explicit gaps-table entries. The php:7.4 chain (Map
passes through, Resolve rejects) proves no silent substitution. The expired
chain (validates clean, plan refuses) proves deploy-guard honesty. Not a UI
change; observable moments are the green suite output, the refusal output,
and the fixture tree.

### Coherence

Diff follows the spec Design and repo conventions: `synthetic` tag mirrors
the `floci` pattern, fixtures mirror the skills-acceptance layout with
original content, tests use table-driven Go asserts with exact error
substrings, no product code touched, no importer features built, no live
calls. `gofmt` clean, `go vet` clean, `make check-clean-room` ok.

## Findings

- SUGGESTION — Direct `file.Resolve("preview")` on an unknown provider
  reports `unknown preset "preview"` instead of naming the provider; the
  supported CLI path (`ResolveBuild`) reports the provider correctly, so
  this is API-message quality only. Owner: config error-text quality
  (`full-deployment-coverage` error-text work).
  `tests/synthetic/suite_test.go:1`
- SUGGESTION — The suite runs via the documented `go test -tags synthetic`
  command only; no Makefile target or CI job wires the tag yet. Owner: any
  future workflow touch (suggested: `reference-store-acceptance` CI
  proofreading).
  `tests/README.md:11`
- SUGGESTION — Follower intents changing offline behavior own this suite's
  updates in their own changes (per spec open questions); this suite pins
  today's honest behavior, not tomorrow's.
  `intent/mocked-shop-scenarios/spec.md:1`

## Not checked

- Full `go test ./...` and `make verify`: scoped to the synthetic suite
  (11/11), `go vet`, `gofmt`, `make check-clean-room`, and the CLI-validate
  probes per serial-build discipline; the full matrix runs in CI.
- Live CI run of this change (no PR opened from here).
- Verified in implementing session (no forked verifier; evidence is suite
  output plus fixture diffs above).

## Verdict

Pass. Mocks prove contracts with original fixtures and loud gaps; nothing
here claims a store. No CRITICAL findings.
