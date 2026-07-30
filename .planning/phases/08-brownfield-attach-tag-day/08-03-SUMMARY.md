---
phase: 08-brownfield-attach-tag-day
plan: 03
subsystem: infra
tags: [aws, rds, adopt, brownfield, database, existing, pulumi]

requires:
  - phase: 08-02
    provides: Spec.Existing.Database + DatabaseSecretARN/DatabaseEndpoint mapping
provides:
  - database.ExistingDatabase + Existing early-return (zero RDS creates)
  - stack.New wiring of Spec.Existing.Database into database.Args.Existing
  - ARN-scoped ECS execution policy grant on adopted MasterSecretARN
affects: [08-04, 08-05, attach-database, apply]

tech-stack:
  added: []
  patterns:
    - "database.Existing mirrors network.Existing reference-without-own"
    - "Stack sets Args.Existing from Spec before database.New; greenfield fields ignored on adopt path"

key-files:
  created: []
  modified:
    - internal/cloud/aws/database/database.go
    - internal/cloud/aws/database/database_test.go
    - internal/cloud/aws/stack/component.go
    - internal/cloud/aws/stack/component_test.go

key-decisions:
  - "Existing path validates identifier/endpoint/secretArn before RegisterComponentResourceV2"
  - "ClusterARN output carries ExternalID as adopt equivalent; Writer/Reader share Endpoint ref"
  - "Assert Existing refs via mock component inputs (literal String ApplyT unreliable when unused)"

patterns-established:
  - "database.New early-returns on Args.Existing before greenfield validate/engine dispatch"
  - "Stack Existing.Database → database.ExistingDatabase{Identifier,Endpoint,SecretARN}"

requirements-completed: [ATTACH-02]

coverage:
  - id: D1
    description: database.New with Existing set creates zero RDS instance/cluster/subnet-group children; component inputs carry operator refs
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/database/ -run 'TestExistingDatabaseUsesRefsWithoutCreatingRDSResources' -count=1
        status: pass
    human_judgment: false
  - id: D2
    description: Invalid Existing refs fail before component/child registration
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/database/ -run 'TestExistingDatabaseRejectsInvalidRefsBeforeRegistration' -count=1
        status: pass
    human_judgment: false
  - id: D3
    description: Stack wires Spec.Existing.Database into database.New; execution policy scopes to adopted secret ARN only
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'TestNewComposesExistingDatabaseWithoutRDSCreates' -count=1
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-30
status: complete
---

# Phase 08 Plan 03: database.Existing + Stack Wiring Summary

**AWS RDS adopt applies via database.Existing reference-without-own; stack feeds Spec adopt refs and keeps execution-role secret access single-ARN scoped**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-30T12:12:13Z
- **Completed:** 2026-07-30T12:16:35Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added `ExistingDatabase` + early-return path that registers outputs from operator refs with zero RDS creates
- Validated identifier/endpoint/Secrets Manager secretArn before registration
- Wired `Spec.Existing.Database` into `database.Args.Existing` while preserving ARN-scoped execution policy

## Task Commits

Each task was committed atomically:

1. **Task 1: database.Existing early-return (zero RDS creates)**
   - `e7f843c` test(08-03): add failing tests for database.Existing
   - `401eb20` feat(08-03): add database.Existing reference-without-own path
2. **Task 2: Stack wires Existing.Database into database.New**
   - `c790334` test(08-03): add failing test for stack Existing.Database wiring
   - `c6b356e` feat(08-03): wire Spec.Existing.Database into database.New

## Files Created/Modified
- `internal/cloud/aws/database/database.go` — ExistingDatabase, validateExistingDatabase, newExistingDatabase
- `internal/cloud/aws/database/database_test.go` — zero-create + reject Existing tests
- `internal/cloud/aws/stack/component.go` — Spec.Existing.Database → database.Args.Existing
- `internal/cloud/aws/stack/component_test.go` — composition test for adopt path + IAM scope

## Decisions Made
- Skip greenfield `validate` when `Args.Existing != nil` (adopt does not need KMS/subnets/engine capacity)
- Reuse stack Secrets Manager ARN regex in the database package for Existing secret validation
- ReaderEndpoint mirrors WriterEndpoint for adopt (single operator endpoint ref)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Assert Existing refs via mock component inputs**
- **Found during:** Task 1 (database.Existing early-return)
- **Issue:** ApplyT on literal `pulumi.String` outputs did not populate capture vars under mocks when unused
- **Fix:** Assert `existingIdentifier` / `existingEndpoint` / `existingSecretArn` on registered component inputs; stack test proves outputs flow into IAM + task definitions
- **Files modified:** `internal/cloud/aws/database/database_test.go`
- **Verification:** `go test ./internal/cloud/aws/database/ -run Existing` and stack ExistingDatabase test pass
- **Committed in:** `401eb20` / `c790334`

**Total deviations:** 1 auto-fixed (Rule 3)
**Impact on plan:** Assertion strategy only; behavior matches plan must-haves.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required. No live AWS (Floci offline only).

## Next Phase Readiness
ATTACH-02 apply path for RDS is closed offline. Ready for 08-04 (remaining attach/tag-day surface). Greenfield Aurora/RDS paths unchanged.

---
*Phase: 08-brownfield-attach-tag-day*
*Completed: 2026-07-30*

## Self-Check: PASSED
