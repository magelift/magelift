---
slug: lean-core-provider-boundary
verified: 2026-09-13
verdict: pass
---

# Report: lean core with one proven subprocess adapter

## What shipped

The host proves signed subprocess Dial for `gcp`/`gke-autopilot` while
everything else stays in-process:

- `Execute` RPC (`preview`, `up`, `destroy`, `outputs`,
  `validate-request`) across `hostproto`, `API`, gRPC plumbing, and
  `Session.Execute`, with 1 MiB wire caps and truncated diagnostics.
  Satisfies stack-lifecycle-in-subprocess.
- `cmd/magelift-provider-gcp` runs `Execute` through Automation API with
  its own `gcpstack` program; `test://` backend URLs select a synthetic
  mock executor for Dial tests. Ownership conflicts return as data and
  the host rebuilds the typed error. Satisfies trust-before-Dial
  execution and ownership parity.
- `Dial` pings and refuses non-`v1` SDK API versions before returning the
  session. Satisfies the handshake scenario (helper-tested; see Findings).
- `SubprocessBackend` adapts a session to `automation.Backend` plus
  `RequestGuard`; `newBackend` selects it for the proof cell when a
  verified artifact is installed, else in-process with a stderr notice.
  Sessions close via defers at all six backend call sites. Satisfies
  CLI-uses-subprocess-when-verified.
- `extensions list` gains a `providers` entry (name, version, digest,
  mode) from a Dial-free `Load`. Satisfies provenance reporting.
- GoReleaser builds `magelift-provider-gcp` on the CLI matrix, uploads
  raw binaries beside the CLI archives, and signs per-binary Cosign
  bundles at release time. Satisfies single-version v1 release.
- ADR 0011 plus `adding-a-provider.md`, ADR 0008, and `versioning.md`
  updates in the same change. Satisfies decision traceability and the
  day-2 gap record.
- Clean-room script asserts zero `internal/` imports for
  `examples/custom-extension-contract`. Satisfies the frozen boundary
  scenario.

## Deviations from plan

- 2.1 took the plan's own executor-seam fallback: Pulumi `WithMocks`
  drives `pulumi.RunErr`, not Automation API stacks, so Dial tests run a
  synthetic mock executor on `test://` backends. Real graph coverage
  stays in the in-process mock suites; real execution waits for Phase 2.
- 1.3 tests the handshake refusal through the `checkAPIVersion` helper
  rather than a Dial-level mismatch (which needs a dedicated fake plugin
  binary). Enforcement lives in `Dial`; the accept path is Dial-tested.
- 4.1 uses `signs` with `artifacts: binary` instead of `binary_signs`,
  which signs at build time and broke the offline smoke (`build --skip`
  cannot skip signing). No smoke script change needed.

## Verification

### Completeness

All 13 plan boxes ticked (1.1 through 5.1). Every spec requirement has
evidence below; two scenarios rest on implementation plus partial tests
(see Findings) rather than direct scenario tests.

### Correctness

- Clean-room extension: `make extension-test` green, including the new
  `go list -deps` zero-`internal/` assertion
  (`/tmp/extension-test-lean-core.log`).
- Trust before Dial: `TestLoadSubprocessRefusesChecksumMismatch`,
  `TestLoadSubprocessRefusesUnsignedVerify`,
  `TestLoadSubprocessRefusesUnknownProvider`, parse refusals, and
  `TestDialPingsVerifiedProvider` green in the providerhost suite.
- Subprocess execution: `TestDialExecutesOverMockBackend` drives all
  five operations plus plan-decode failure propagation over a real
  Dialed subprocess; `TestSubprocessBackend*` (5 tests) pins the
  adapter mapping, diagnostics, ownership rebuild, outputs, and error
  propagation.
- CLI branches: 6 new CLI tests pin subprocess selection, fallback
  notice, non-proof silence, provenance, `extensions list` output, and
  missing-lockfile refusal. Full `internal/cli` suite: 268 pass, 0 fail.
- Human-observable moment: built `/tmp/magelift-lean-core` from this
  tree. `extensions list -o json` prints the providers entry with
  `magelift-provider-gcp` in `in-process` mode (no lockfile beside the
  binary); `-o bogus` prints the usage error with exit 2. A `preview`
  run against a GCP fixture reached plan admission (blocked on expired
  local GCP credentials, unrelated to this change), so the fallback
  notice was not observed through the real binary and rests on unit
  tests.
- Lock stays with host: lock code untouched by the diff; existing lock
  tests pass in the full CLI suite; session cleanup defers run on error
  paths too.
- Secrets: `Execute` carries operations, plans, backend identity, and
  identifier-only preview metadata; outputs use `RedactedOutputs`;
  no RPC payload logging added (reviewed, not executed).
- Day-2 gap and decision: ADR 0011 states the gap, the proof scope, and
  the post-v1 order; `adding-a-provider.md` and `versioning.md` updated;
  `make docs` strict build green (`/tmp/docs-lean-core.log`).
- Release: `goreleaser check` validates the config; `make release-smoke`
  green single-target with both binaries in `dist/`
  (`/tmp/release-smoke-lean-core2.log`).
- Constraint check: `internal/cli` gains no direct Pulumi SDK import;
  `internal/automation` already links 87 Pulumi packages, so the
  providerhost edge adds no new SDK family, and the CLI suite links
  and runs on this small runner.

### Coherence

The diff follows the spec Design: `providerhost` owns trust plus
transport, `gcp/stack` owns plan and program, the CLI owns approvals,
lock, and sessions, release engineering owns the signed artifact.
Caps, `AutoMTLS`, gRPC-only protocol, and the handshake are untouched.
`Load`-then-`Dial` ordering holds at the one call site, and no path
shells out to an unverified binary. Single-version comment in
`.goreleaser.yaml` matches ADR 0011.

## Findings

- WARNING — no test pins subprocess `Plan` equality with in-process
  `Plan` for the same input (spec scenario "Autopilot plan round trip").
  Equality holds by construction (the subprocess runs the same
  `gcpstack.Module.Plan`), but the `configFromMap` drift surface is
  unpinned. `internal/providerhost/host_test.go:172`
- SUGGESTION — Dial-level handshake-mismatch negative test missing;
  enforcement is helper-tested only. `internal/providerhost/session.go:86`
- SUGGESTION — subprocess crash mid-apply surfaces the RPC error but
  prints no resume guidance (spec gotcha). The in-process path has no
  resume guidance either, so this matches current behavior.
  `internal/cli/subprocess.go:120`

## Not checked

- Verified in implementing session (no separate verifier available).
- Live subprocess activation (preview, deploy, outputs, destroy through
  a Dialed session on Autopilot): needs a release-signed bundle plus
  GCP credentials; explicitly Phase 2 per the intent and ADR 0011.
- `magelift version` versus provider version match at a real tag;
  mechanism is unit-tested, tag-time observation pending.
- Fallback notice through the real CLI binary; blocked on expired local
  GCP credentials at plan admission, covered by unit tests instead.
- Windows `.exe` asset naming and bundle upload; verified on first tag
  release per ADR 0011.
- Whether any third party has built a community module from the docs
  alone (carried over from the intent).

## Verdict

Pass. The boundary is frozen, Dial is proven for one adapter up to the
live run, and the remaining gaps are tests and tag-time observations,
not behavior. Suggested follow-ups (plan-equality test, Dial-level
mismatch test) fit the next change touching `internal/providerhost`.
