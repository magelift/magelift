---
status: done
slug: onboarding-migration-presets
spec: spec.md
---

# Plan: install, import or preset, deploy

Auto-approved per the standing `/goal` instruction.

## Files that change

- EDIT `internal/cli/root.go`: `init --provider` flag, two starter
  constants, provider validation before overwrite check.
- EDIT `internal/paasimport/unmapped.go`: family guidance section
  plus hook before/after in `RenderUnmappedReport`.
- EDIT `internal/cli/doctor.go`: `Next` from first failed check.
- EDIT `docs/getting-started.md`: starter shapes plus command.
- EDIT `agents/skills/magelift-configure/SKILL.md` and
  `agents/skills/magelift-migrate/SKILL.md` only if stale.
- EDIT tests: `init_import_test.go` (provider starters),
  `paasimport` sidecar tests, `doctor_test.go` (Next on failure).

## Order of work

- [x] 1.1 `init --provider` with AWS plus GCP starters, each with
  three envs — verify: new CLI tests write both starters to temp
  dirs and `config validate` passes on each
- [x] 1.2 Sidecar checklist with hook before/after, no value echo —
  verify: importer unit tests assert guidance plus absence of input
  values
- [x] 1.3 `doctor` Next on failure — verify: unit tests for build,
  environment, and dependency first-failures
- [x] 1.4 Getting-started shapes plus skills touch-up — verify:
  `make docs` strict green
- [x] 1.5 End-to-end drive: init both providers, validate, doctor,
  import fixture with residuals — verify: commands exit as
  specified; failures recorded, not edited around

## Risks

- GCP starter field drift: build it from the CI fixture shape and
  let `config validate` in tests catch drift.
- Doctor Next map goes stale as checks grow: default Next points at
  `config validate`; unknown check IDs stay covered.

## Proof

Starter validate output, sidecar sample, doctor Next samples, docs
build log, full CLI plus paasimport suites green.
