---
status: planned
slug: pulumi-error-causes
spec: spec.md
---

# Plan: keep Pulumi update causes, including concurrent updates

Auto-approved per the standing `/goal` instruction.

## Files that change

- NEW `internal/automation/concurrent_update.go`: typed error plus
  shared `IsConcurrentUpdate` helper.
- EDIT `internal/automation/runner.go`: wrap causes, classify locks.
- NEW `internal/automation/concurrent_update_test.go`: wrap plus
  classification matrix on a fake backend.
- EDIT `internal/cli/lifecycle.go` (or `root.go` boundary): map the
  typed error to a named exit code plus retry sentence.
- EDIT `internal/cli/lifecycle_test.go` (or new): CLI code test.
- EDIT `internal/providerhost/execute.go`: `concurrentUpdate` wire
  flag on `ExecuteResult`.
- EDIT `cmd/magelift-provider-gcp/execute.go`: set the flag via the
  shared helper, next to the ownership check.
- EDIT `internal/providerhost/subprocess_backend.go`: rebuild the
  typed error from the flag.
- EDIT `internal/providerhost/subprocess_backend_test.go` (or new):
  wire round-trip test via the mock backend.

## Order of work

- [x] 1.1 Typed error plus helper plus Runner wiring — verify:
  new automation tests pin wrapping and classification
- [x] 1.2 CLI exit mapping plus test — verify: lifecycle test
  asserts code and sentence
- [x] 1.3 Subprocess wire flag, set plus rebuild, plus round-trip
  test — verify: providerhost test pins host-side typing
- [x] 1.4 Full suite plus lint — verify: `go test ./...` green,
  linter clean on touched packages

## Risks

- Exit code collision: check existing named codes before picking
  the const value.

## Proof

Unit and mock tests, suite log, lint log.
