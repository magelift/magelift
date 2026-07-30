---
phase: 08-brownfield-attach-tag-day
plan: 02
subsystem: infra
tags: [aws, rds, adopt, brownfield, config, schema, existing-database]

requires:
  - phase: 08-01
    provides: ExistingResources network adopt pattern and refuse gate
provides:
  - AWSExistingDatabase config surface (provider/kind/externalId/secretArn/endpoint)
  - Spec.Existing.Database + DatabaseSecretARN/DatabaseEndpoint mapping via PlanFromConfig
  - Validation rejecting incomplete/wrong-kind database adopt refs (Secrets Manager ARN only)
affects: [08-03, 08-04, 08-05, attach-database]

tech-stack:
  added: []
  patterns:
    - "Dedicated AWSExistingDatabase struct (not overloaded AWSExistingResource)"
    - "Nested secretArn/endpoint under existing.database; no inline passwords"
    - "PlanFromConfig → Spec.Existing.Database before any Pulumi component work"

key-files:
  created: []
  modified:
    - internal/config/model.go
    - internal/config/schema.go
    - internal/config/schema_test.go
    - docs/configuration.md
    - schema/magelift.schema.json
    - internal/cloud/aws/stack/spec.go
    - internal/cloud/aws/stack/config.go
    - internal/cloud/aws/stack/config_test.go
    - internal/cloud/aws/stack/spec_test.go

key-decisions:
  - "AWSExistingDatabase dedicated struct with nested secretArn/endpoint (D-01 discretion)"
  - "Spec fields DatabaseSecretARN/DatabaseEndpoint colocated with Database for 08-03 wiring"
  - "Accept Secrets Manager ARN only; reject incomplete refs before Pulumi"

patterns-established:
  - "existing.database mirrors existing.network: config → PlanFromConfig → Spec.Existing → later component Existing branch"
  - "Incomplete adopt refs fail with 'existing database requires …' at Validate/PlanFromConfig"

requirements-completed: [] # ATTACH-02 surface only; apply closes in 08-03

coverage:
  - id: D1
    description: magelift.yaml can declare target.aws.existing.database with provider/kind/externalId/secretArn/endpoint; schema and configuration reference document it
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/config/ -run 'TestExistingDatabaseAppearsInSchemaAndReference' -count=1
        status: pass
      - kind: other
        ref: make generate-check
        status: pass
    human_judgment: false
  - id: D2
    description: PlanFromConfig maps complete existing.database into Spec.Existing.Database plus secret/endpoint fields
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'TestPlanFromConfigMapsExistingDatabaseInputs' -count=1
        status: pass
    human_judgment: false
  - id: D3
    description: Incomplete or wrong-kind database adopt refs are rejected; network-only configs still plan
    requirement: ATTACH-02
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'PlanFromConfigRejectsIncompleteExistingDatabase|PlanFromConfigRejectsWrongExistingDatabaseKind|PlanFromConfigKeepsExistingNetworkOnly|TestSpecValidateAcceptsExistingDatabase' -count=1
        status: pass
    human_judgment: false

duration: 8min
completed: 2026-07-30
status: complete
---

# Phase 08 Plan 02: existing.database Config/Spec Summary

**Operators can declare AWS RDS MySQL adopt refs in magelift.yaml (`existing.database` + secretArn/endpoint); PlanFromConfig validates them into Spec without creating cloud resources**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-30T12:08:05Z
- **Completed:** 2026-07-30T12:16:00Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Added `AWSExistingDatabase` under `target.aws.existing.database` with required secretArn (Secrets Manager ARN only) and writer endpoint
- Regenerated `docs/configuration.md` / `schema/magelift.schema.json` with adopt-without-create RDS prose
- Mapped complete refs into `Spec.Existing.Database` + `DatabaseSecretARN` / `DatabaseEndpoint`; reject incomplete and wrong-kind refs

## Task Commits

Each task was committed atomically:

1. **Task 1: Add existing.database config model + schema prose**
   - `0dfcfa6` test(08-02): add failing test for existing.database schema
   - `4e1e1d0` feat(08-02): add existing.database config model and schema
2. **Task 2: Map PlanFromConfig + Spec validation for existing database**
   - `084041a` test(08-02): add failing tests for existing.database plan mapping
   - `ca48855` feat(08-02): map and validate existing.database in Spec

## Files Created/Modified
- `internal/config/model.go` — `AWSExistingDatabase` + `AWSExistingResources.Database`
- `internal/config/schema.go` — existing.database prose (no RDS create; no Cloud SQL key)
- `internal/config/schema_test.go` — schema/reference assertions for database fields
- `docs/configuration.md` / `schema/magelift.schema.json` — regenerated
- `internal/cloud/aws/stack/spec.go` — ExistingResources database fields + validate
- `internal/cloud/aws/stack/config.go` — PlanFromConfig mapping
- `internal/cloud/aws/stack/config_test.go` / `spec_test.go` — map/reject/accept coverage

## Decisions Made
- Dedicated `AWSExistingDatabase` struct (not extending `AWSExistingResource`) per D-01 discretion
- Spec uses `DatabaseSecretARN` / `DatabaseEndpoint` colocated with `Database` for 08-03 component wiring
- No Pulumi database.Existing branch in this plan (08-03); no live AWS

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Config/Spec surface ready for 08-03 `database.Existing` component path and stack wiring
- Network-only adopt path unchanged; ATTACH-02 apply still pending 08-03

## Self-Check: PASSED

- FOUND: internal/config/model.go (AWSExistingDatabase)
- FOUND: Spec.Existing.Database + DatabaseSecretARN/DatabaseEndpoint
- FOUND commits: 0dfcfa6, 4e1e1d0, 084041a, ca48855
- Verified: `go test ./internal/config/` (49 pass), `go test ./internal/cloud/aws/stack/` (71 pass), `make generate-check`

---
*Phase: 08-brownfield-attach-tag-day*
*Completed: 2026-07-30*
