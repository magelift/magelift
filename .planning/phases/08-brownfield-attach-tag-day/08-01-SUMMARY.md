---
phase: 08-brownfield-attach-tag-day
plan: 01
subsystem: infra
tags: [aws, vpc, adopt, brownfield, pulumi, refuse-before-mutate]

requires:
  - phase: existing-network-import
    provides: target.aws.existing.network reference-without-own path
provides:
  - AdoptReport for existing VPC with operator-visible ADOPT lines
  - RefuseAdoptedMutation gate naming adopted network externalId
  - CLI preview/deploy/destroy surfaces adopted entries
affects: [08-02, 08-03, 08-04, attach-database]

tech-stack:
  added: []
  patterns:
    - "platform.BrownfieldAttach optional interface avoids CLI→pulumi-aws import"
    - "ADOPT {kind} {externalId} stderr + infrastructureResult.adopted"
    - "RefuseAdoptedMutation(destroy|replace) fail-closed; Magento-scoped ops use empty intent"

key-files:
  created:
    - internal/cloud/aws/stack/adopt.go
    - internal/cloud/aws/stack/adopt_test.go
    - internal/platform/attach.go
  modified:
    - internal/cli/lifecycle.go
    - internal/cli/lifecycle_test.go
    - internal/cli/stub_module_test.go
    - internal/cloud/aws/network/network_test.go

key-decisions:
  - "BrownfieldAttach on PlannedStack so CLI stubs can surface ADOPT without importing awsstack/pulumi-aws"
  - "Stack-scoped deploy/destroy pass empty refuse intent so detach-via-destroy remains valid; destroy/replace intents fail closed"
  - "Database adopt fields deferred to 08-02/03"

patterns-established:
  - "AdoptReport(spec) → AdoptEntry.Line() for stable ADOPT preview contract"
  - "announceAdoptedResources + refuseAdoptedMutationForOperation before provider mutate"

requirements-completed: [ATTACH-01, ATTACH-03]

coverage:
  - id: D1
    description: Preview reports ADOPT network line with VPC externalId when Existing.Network is set
    requirement: ATTACH-01
    verification:
      - kind: unit
        ref: go test ./internal/cli/ -run 'PreviewReportsAdoptNetwork' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'AdoptReport' -count=1
        status: pass
    human_judgment: false
  - id: D2
    description: RefuseAdoptedMutation fails before mutate naming resource and externalId for destroy/replace intent
    requirement: ATTACH-03
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'RefuseAdoptedMutation' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cli/ -run 'InfrastructureRefuseAdopted' -count=1
        status: pass
    human_judgment: false
  - id: D3
    description: Existing network mock graph creates zero VPC/subnet/NAT children
    requirement: ATTACH-01
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/network/ -run 'ExistingNetworkUsesImported' -count=1
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-07-30
status: complete
---

# Phase 08 Plan 01: VPC ADOPT Preview + Refuse-Before-Mutate Summary

**Operator-visible ADOPT lines for existing VPC plus named refuse-before-mutate for destroy/replace intents, with zero managed VPC creates preserved.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-30T12:01:29Z
- **Completed:** 2026-07-30T12:07:01Z
- **Tasks:** 2/2
- **Files modified:** 7

## Accomplishments
- `AdoptReport` emits `ADOPT network {vpcId}` from `Spec.Existing.Network`; CLI preview/deploy/destroy write lines to stderr and `infrastructureResult.adopted`
- `RefuseAdoptedMutation` fails closed on destroy/replace intent with `adopted resource {label} ({externalId}): MageLift does not own this resource`
- Existing-network mock graph still creates zero `aws:ec2/vpc:Vpc` / subnet / NAT children

## Task Commits

1. **Task 1: End-to-end VPC ADOPT report + refuse gate** - `d581791` (feat)
2. **Task 2: Regress Existing network zero-create graph** - `4aae654` (test)

## Files Created/Modified
- `internal/platform/attach.go` - BrownfieldAttach + AdoptMutationIntent
- `internal/cloud/aws/stack/adopt.go` - AdoptReport, RefuseAdoptedMutation, Planned adapters
- `internal/cloud/aws/stack/adopt_test.go` - report/refuse/interface coverage
- `internal/cli/lifecycle.go` - ADOPT announce + refuse consult on deploy/destroy
- `internal/cli/lifecycle_test.go` - PreviewReportsAdoptNetwork + refuse naming test
- `internal/cli/stub_module_test.go` - stub BrownfieldAttach from existing.network config
- `internal/cloud/aws/network/network_test.go` - ATTACH-01 zero-create regression note

## Decisions Made
- CLI talks to `platform.BrownfieldAttach` so unit tests keep the stub module (no pulumi-aws link) while production AWS `Planned` implements the same contract via `AdoptReport`/`RefuseAdoptedMutation`
- Magento-scoped deploy/destroy pass empty refuse intent (detach-via-stack-destroy stays valid per D-03); explicit destroy/replace intents against adopted VPC fail closed (D-02)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Added platform.BrownfieldAttach port**
- **Found during:** Task 1
- **Issue:** CLI importing `internal/cloud/aws/stack` would pull pulumi-aws into CLI unit tests that intentionally stub AWS
- **Fix:** Optional `platform.BrownfieldAttach` implemented by AWS `Planned` and CLI stub; lifecycle announces/refuses via the interface
- **Files modified:** `internal/platform/attach.go`, `internal/cli/stub_module_test.go`, `internal/cli/lifecycle.go`
- **Verification:** CLI Preview|Adopt|Infrastructure tests green under GOMAXPROCS=1
- **Committed in:** `d581791`

---

**Total deviations:** 1 auto-fixed (Rule 2)
**Impact on plan:** Necessary for CLI test isolation; same ADOPT/refuse contracts as planned.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required. No live AWS create (Floci/mocks only).

## Next Phase Readiness
Network ADOPT + refuse slice ready for 08-02/03 database adopt and 08-04 unified report.

## Self-Check: PASSED
- FOUND: internal/cloud/aws/stack/adopt.go
- FOUND: internal/cloud/aws/stack/adopt_test.go
- FOUND: internal/platform/attach.go
- FOUND: d581791
- FOUND: 4aae654

---
*Phase: 08-brownfield-attach-tag-day*
*Completed: 2026-07-30*
