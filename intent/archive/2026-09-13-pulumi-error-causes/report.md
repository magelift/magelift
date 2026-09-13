---
status: verified
slug: pulumi-error-causes
plan: plan.md
verdict: pass
---

# Report: keep Pulumi update causes, including concurrent updates

## What shipped

- `Runner.run` wraps every backend cause under the operation
  sentinel; ownership errors keep their shape.
- Typed `automation.ConcurrentUpdateError` plus `IsConcurrentUpdate`
  (Pulumi's own predicate, then the documented lock fragments, since
  `autoError` has no exported constructor for mocks).
- Causes pass through the shared secret redactor before entering
  the returned chain: classification on raw text, only redacted
  text printable. The redactor moved to `internal/secretsafe` with
  `certification` aliases so the leaf automation package shares
  the single definition; the old total-suppression test now pins
  redact-not-drop.
- CLI maps collisions on preview/deploy/destroy to exit 5 with a
  wait-and-retry sentence; documented in `docs/operations.md`.
- Subprocess parity: `ExecuteResult.concurrentUpdate` wire flag,
  set by the GCP provider via the shared helper, rebuilt by
  `SubprocessBackend` so the exit-5 mapping fires through Dial.

## Deviations

One reconciliation, recorded in the spec: the pre-existing
redaction test asserted total suppression, which contradicts the
intent's visible-cause outcome. The constraint is now read against
the codebase's own definition of secret material
(credential-shaped text), redacted via the shared helper; ordinary
cause text stays visible.

## Evidence

- `go test ./... -count=1`: exit 0, 133 packages ok
  (`/tmp/test-error-causes.log`).
- Targeted: automation plus providerhost plus gcp-cmd 64 passed;
  CLI collision tests 2 passed.
- Linter on all touched packages: 0 issues
  (`/tmp/lint-error-causes.log`); gofmt clean; `make docs`: exit 0
  (`/tmp/docs-error-causes.log`).

## Verdict

Pass. All five spec scenarios hold. A live two-deploy collision
stays unchecked until the Phase 2 cloud window, as specced.
