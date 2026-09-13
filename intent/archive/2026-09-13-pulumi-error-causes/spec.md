---
status: specified
slug: pulumi-error-causes
intent: intent.md
---

# Spec: keep Pulumi update causes, including concurrent updates

Auto-approved per the standing `/goal` instruction. Defaults taken:
concurrent-update gets a dedicated exit code (generated CI needs a
machine-readable retry signal, and a bare wrapped message still reads
as a graph bug to operators without DevOps background); preview,
update, and destroy share the same classification because they share
`Runner.run`.

## Requirements

### Requirement: backend causes survive

`Runner.run` SHALL wrap the backend cause under the operation
sentinel for preview, update, and destroy, instead of returning the
bare sentinel. Ownership errors keep their existing shape.

#### Scenario: update failure names the cause

- **WHEN** the backend update fails with a graph error
- **THEN** the returned error matches `ErrUpdateFailed` via
  `errors.Is` and its message contains the backend cause

### Requirement: concurrent updates classify

`Runner.run` SHALL return a typed concurrent-update error, matchable
via `errors.As`, when the backend cause is a Pulumi stack lock or
409 conflict. Detection uses Pulumi's own
`auto.IsConcurrentUpdateError` plus the two documented stderr
fragments as fallback (`autoError` has no exported constructor, so
mocks cannot build a real one).

#### Scenario: colliding deploy classifies

- **WHEN** the backend fails with a lock/409 cause
- **THEN** the returned error matches the typed concurrent-update
  error and still matches the operation sentinel

### Requirement: CLI exits distinct on collision

`preview`, `deploy`, and `destroy` SHALL exit with a dedicated
concurrent-update code and an operator sentence (another deployment
holds the stack; wait, then retry) instead of the generic failure.

#### Scenario: deploy collision exits distinct

- **WHEN** deploy hits a concurrent-update cause
- **THEN** the CLI exits with the dedicated code and prints the
  retry sentence

### Requirement: classification crosses the subprocess boundary

The subprocess proof cell SHALL classify lock/409 failures the same
way: the provider subprocess detects via the shared helper, signals
it on `ExecuteResult`, and `SubprocessBackend` rebuilds the typed
error so the CLI mapping still fires.

#### Scenario: subprocess collision classifies

- **WHEN** the GCP provider subprocess hits a lock/409 cause
- **THEN** the host-side error matches the typed concurrent-update
  error

### Requirement: mocks cover the classification

Every path above SHALL be covered by Pulumi mock tests (fake
backend, fake lock text, mock Execute backend). No live cloud.

#### Scenario: suite pins the matrix

- **WHEN** the suite runs
- **THEN** tests pin cause wrapping, lock classification,
  the CLI code, and the subprocess round trip

## Design

- New file `internal/automation/concurrent_update.go`: typed error
  plus `IsConcurrentUpdate(err)` (auto predicate, then documented
  fragments). `Runner.run` checks ownership first, then lock, then
  wraps generic.
- CLI maps via `errors.As` at the infrastructure command boundary
  to a named exit code const.
- `ExecuteResult` gains a `concurrentUpdate` wire flag; gcp
  `execute.go` sets it next to the ownership check;
  `SubprocessBackend` rebuilds the typed error.

## Gotchas / policy flags

- Classification runs on the raw backend cause, but only text passed
  through the shared `internal/secretsafe` redactor enters the
  returned chain (single definition of credential-shaped material,
  re-exported by `certification`). Ordinary cause text stays fully
  visible; the old total-suppression test now pins redact-not-drop.
- The fallback fragments are copied from Pulumi's own matcher;
  comment their source so a Pulumi bump re-checks them.
- Do not retry locked stacks; classification only.

## Open questions carried forward

None. Both intent questions are decided above; a live two-deploy
collision stays unchecked until the Phase 2 cloud window.
