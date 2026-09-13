---
status: verified
slug: dial-proof-followups
plan: plan.md
verdict: pass
---

# Report: close the Dial proof test gaps

## What shipped

Test-only intent; no behavior change.

- `TestDialPlanEqualsInProcessPlan`: a Dialed GCP subprocess Plan
  equals the in-process `gcpstack.Module.Plan` for the same staging
  fixture, compared as decoded JSON including the opaque spec.
  Mutation-checked (fails on a tampered region, then reverted).
- `TestDialRefusesVersionMismatch`: Dialing a helper plugin that
  pings `v0-test` fails with `ErrUnsupportedAPI` and no session,
  via a `TestMain` re-exec helper. Closes the archived report's
  WARNING plus SUGGESTION.
- Resume guidance stays out on both paths (recorded decision:
  subprocess-only guidance would be inconsistent with in-process).

## Deviations

None. One test-authoring slip (unhashable map key in the comparison
loop) was caught by the test run itself and rewritten plainly.

## Evidence

- `go test ./internal/providerhost/ -count=1`: exit 0
  (`/tmp/test-dial-followups.log`).
- Mutation run: FAIL as expected, then revert plus green re-run.
- Linter on `internal/providerhost/...`: 0 issues
  (`/tmp/lint-dial-followups.log`); gofmt clean.

## Verdict

Pass. Roadmap order 6 is now fully closed up to the live run, which
stays Phase 2 per ADR 0011.
