---
phase: 01-publishable-baseline-honest-fallbacks
plan: 09
subsystem: cli
tags: [trust-01, trust-02, experimental-warning, infra-only, deploy-refusal, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: stubExperimentalModule / stubExperimentalAWSEKSModule; usererr; --infra-only flag; planStack choke point
provides:
  - Tier-keyed experimental warning at planStack before any backend
  - Deploy refuses Magento-less updates without --infra-only; announces when flagged
  - Verbatim warning / refusal / notice strings for Phase 3 evidence and Phase 6 day-2
affects: [TRUST-01, TRUST-02, Phase 3 evidence tiering, Phase 6 day-2, plan 01-10 AcquireLock]

tech-stack:
  added: []
  patterns:
    - "warnExperimentalTarget at planStack — tier check, stderr only, no prompt"
    - "refuseInfraOnlyDeploy via usererr + exit 2; announceInfraOnlyDeploy on --infra-only"
    - "Gate on CertificationTier / ErrNotSupported — never provider allowlists"

key-files:
  created:
    - internal/usererr/usererr.go
    - internal/usererr/usererr_test.go
  modified:
    - internal/cli/lifecycle.go
    - internal/cli/lifecycle_test.go
    - internal/cli/stub_module_test.go
    - internal/cli/cost_test.go
    - docs/capability-matrix.md

key-decisions:
  - "Warn unconditionally at planStack (read-only included) — mutating-only flag would regress silently"
  - "Key warning on CertificationTier so aws/eks-autopilot cannot slip past a provider allowlist"
  - "Refuse Magento-less deploy by default; --infra-only is the explicit proceed path (criterion 5 stricter reading)"

patterns-established:
  - "stderr for honesty warnings/notices; stdout for structured payloads"
  - "usererr.New(cause, next, doc) for actionable CLI refusals"

requirements-completed: [TRUST-01, TRUST-02]

coverage:
  - id: D1
    description: Experimental targets warn at planStack before backend; certified targets stay silent; AWS EKS experimental covered
    requirement: TRUST-01
    verification:
      - kind: unit
        ref: "TestExperimentalTargetWarnsAtPlanStack + TestExperimentalWarningLeavesJSONStdoutParseable"
        status: pass
    human_judgment: false
  - id: D2
    description: Deploy without Magento steps refuses without --infra-only; with flag succeeds and announces; full flow and genuine errors distinct
    requirement: TRUST-02
    verification:
      - kind: unit
        ref: "TestDeployRefusesInfraOnlyWithoutFlag / TestDeployInfraOnlyFlagAnnouncesSkip / TestDeployFullFlowUnaffectedWhenStepsExist / TestDeployGenuineStepsConstructionErrorDiffersFromRefusal"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 09: Experimental Warning & Deploy Refusal Summary

**planStack warns on experimental tiers before any backend; deploy refuses silent Magento-less success unless `--infra-only` is explicit.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-28T13:54:49Z
- **Completed:** 2026-07-28T13:57:30Z
- **Tasks:** 2/2
- **Files modified:** 6

## Accomplishments

- TRUST-01 criterion 4: every command through `planStack` emits a tier-named stderr warning for experimental targets (including `aws/eks-autopilot`) and stays silent for certified ones; JSON stdout remains parseable.
- TRUST-02 criterion 5 (first silent-success path): Magento-less `deploy` exits non-zero without `--infra-only`; with the flag it updates infra and announces skipped migrate/cutover/health.
- Capability matrix documents the `--infra-only` requirement for targets without Magento deploy Ops.

## Task Commits

Each task was committed atomically:

1. **Task 1: Warn at the planning choke point whenever the target is experimental** - `d471d05` (feat)
2. **Task 2: Stop `deploy` from reporting success for an infrastructure-only update** - `9a75f53` (test/RED) → `54fdcf6` (feat/GREEN)

**Plan metadata:** (pending docs commit)

## Verbatim operator-facing strings

Recorded for Phase 3 evidence tiering and Phase 6 day-2 wording:

**Experimental warning (stderr, fmt with provider/runtime):**

```
warning: target %s/%s is experimental: day-2 operations may be unimplemented and this target has no real-account acceptance evidence; see docs/capability-matrix.md
```

Example: `warning: target ovh/mks is experimental: day-2 operations may be unimplemented and this target has no real-account acceptance evidence; see docs/capability-matrix.md`

**Deploy refusal (usererr cause + Next + Docs):**

```
deploy on target %s/%s (%s) cannot run Magento migrate, cutover, and health
Next: Re-run with --infra-only to update the infrastructure graph only.
Docs: docs/capability-matrix.md
```

**Infra-only notice (stderr):**

```
notice: --infra-only: Magento migrate, cutover, and health were skipped
```

## Files Created/Modified

- `internal/cli/lifecycle.go` — `warnExperimentalTarget` in `planStack`; `refuseInfraOnlyDeploy` / `announceInfraOnlyDeploy` in deploy path
- `internal/cli/lifecycle_test.go` — both-direction tier warnings; four deploy infra-only behaviours
- `internal/cli/stub_module_test.go` — `stubExperimentalAWSEKSModule` for provider-vs-tier gate
- `internal/cli/cost_test.go` — eks-autopilot registration note for tier tests
- `internal/usererr/usererr.go` / `usererr_test.go` — structured CLI errors (tracked with Task 2)
- `docs/capability-matrix.md` — `--infra-only` requirement for Magento-less targets

## Decisions Made

- Warn on every `planStack` call (including read-only): over-warning on experimental is correct; a mutating-only flag across eighteen call sites is a silent regression risk.
- Key the warning on `CertificationTier()`, never a provider list — covers experimental AWS Kubernetes.
- Criterion 5 stricter reading: refuse by default; proceed only with explicit `--infra-only` (warning-only would still green unattended pipelines).

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written.

### Notes

- Task 1 arrived as a single `feat` commit (tests + implementation together) from a prior stalled dispatch resume; Task 2 followed RED→GREEN.
- Hosted CI / ROADMAP criterion 1 remains deferred per `.planning/loop/HUMAN_GATE` (Actions minutes exhausted; local `act` substitute only).

## TDD Gate Compliance

- Task 1: no separate `test(...)` RED commit observed — combined feat (resume). Warning logged.
- Task 2: RED `9a75f53` then GREEN `54fdcf6` — compliant.

## Known Stubs

None.

## Threat Flags

None beyond plan threat model mitigations T-01-33…T-01-37 (warning content, stderr/stdout split, refuse path).

## Offline / CI note

Phase 1 **plan execution** complete offline. Hosted green-on-main (ROADMAP criterion 1) still open until Actions minutes return; use `make ci-act-go` / HUMAN_GATE locally — do not dispatch GitHub Actions.

## Verification results

```
GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cli/ -run 'Deploy|InfraOnly' -count=1  → PASS
GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cli/ ./internal/deploy/ -count=1     → PASS
```

## Self-Check: PASSED
