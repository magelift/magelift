---
phase: 06-shared-kubernetes-day-2
plan: 01
subsystem: infra
tags: [s3, sse, aes256, kms, ovh, scaleway, floci, state-lock]

requires:
  - phase: 05-data-migration-cutover
    provides: platform.State port and AWS DIY lock/archive baseline
provides:
  - ObjectEncryption SSE policy (KMS / AES256 / None)
  - OVH and Scaleway concrete State adapters via NewAWSWithEndpoint
  - Offline AES256 unit proof (+ Floci AES256 test under build tag)
affects:
  - 06-05 honesty / AcquireLock hard-fail flip
  - KUBE-05 DIY state certification

tech-stack:
  added: []
  patterns:
    - ObjectEncryption drives PutObject/CopyObject SSE instead of hard-coded KMS
    - OVH/SCW State reuses aws/state + endpoint.Parse loopback gate

key-files:
  created:
    - internal/cloud/aws/state/encryption.go
    - internal/cloud/ovh/stack/state.go
    - internal/cloud/ovh/stack/state_test.go
    - internal/cloud/scaleway/stack/state.go
    - internal/cloud/scaleway/stack/state_test.go
    - tests/floci/state_aes256_test.go
  modified:
    - internal/cloud/aws/state/lock.go
    - internal/cloud/aws/state/archive.go
    - internal/cloud/aws/state/client.go
    - internal/cloud/aws/ops/day2.go
    - internal/cloud/aws/ops/module.go
    - internal/cloud/aws/eksops/ops.go
    - internal/config/model.go
    - schema/magelift.schema.json
    - docs/configuration.md

key-decisions:
  - "ObjectEncryption modes: EncryptionKMS (AWS), EncryptionAES256 (OVH/SCW/Floci), EncryptionNone only as Floci SSE-reject fallback"
  - "OVH/SCW State requires target.*.stateBucket; endpoint from stateEndpoint or MAGELIFT_AWS_ENDPOINT_URL via endpoint.Parse"
  - "AES256 proven offline via unit fake S3API; Floci AES256 test present but skipped unless MAGELIFT_FLOCI=1"

patterns-established:
  - "NewManager/NewArchiveFromClient/NewAWS* take ObjectEncryption, not bare kmsARN"
  - "Provider State adapters are thin wrappers over aws/state — no ovh/state or scaleway/state packages"

requirements-completed: [KUBE-05]

coverage:
  - id: D1
    description: ObjectEncryption drives DIY PutObject/CopyObject SSE; AWS callers stay on EncryptionKMS
    requirement: KUBE-05
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/state/ -count=1 -run 'Encryption|AES256|Manager|Archive|Lock'
        status: pass
    human_judgment: false
  - id: D2
    description: OVH and Scaleway Module.State() return concrete adapters using NewAWSWithEndpoint + EncryptionAES256
    requirement: KUBE-05
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'State|AES256|Encryption|DIY'
        status: pass
    human_judgment: false
  - id: D3
    description: Offline AES256 lock path proven (unit); Floci AES256 coverage under build tag floci
    requirement: KUBE-05
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/state/ -count=1 -run 'AES256'
        status: pass
      - kind: integration
        ref: tests/floci/state_aes256_test.go (skipped unless MAGELIFT_FLOCI=1)
        status: unknown
    human_judgment: false

duration: 6min
completed: 2026-07-30
status: complete
---

# Phase 6 Plan 01: ObjectEncryption + OVH/SCW State Summary

**One shared S3-compatible state manager with ObjectEncryption (AWS KMS preserved; OVH/SCW AES256 + endpoint override) proven offline.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-07-30T10:41:08Z
- **Completed:** 2026-07-30T10:46:46Z
- **Tasks:** 2/2
- **Files modified:** 16 (task 2) + 10 (task 1)

## Accomplishments

- Introduced `ObjectEncryption` with KMS / AES256 / None modes driving SSE on lock and archive writes
- Migrated AWS ECS + EKS State/archive constructors to `EncryptionKMS`
- Wired OVH and Scaleway `Module.State()` to concrete adapters over `NewAWSWithEndpoint` + `EncryptionAES256` (no forked state packages)
- Added `stateBucket` / `stateEndpoint` / `stateRegion` YAML keys + schema regenerate
- AES256 unit lock/archive tests green; Floci AES256 test added (skips without `MAGELIFT_FLOCI=1`)

## Task Commits

1. **Task 1: End-to-end AES256 lock PutObject via ObjectEncryption** - `e90d629` (feat)
2. **Task 2: Wire OVH/SCW State through NewAWSWithEndpoint + Floci AES256** - `449ad85` (feat)

## Files Created/Modified

- `internal/cloud/aws/state/encryption.go` — ObjectEncryption API + applyPut/applyCopy
- `internal/cloud/aws/state/lock.go` / `archive.go` / `client.go` — constructors take ObjectEncryption
- `internal/cloud/ovh/stack/state.go` / `scaleway/stack/state.go` — concrete State adapters
- `tests/floci/state_aes256_test.go` — Floci AES256 (with EncryptionNone fallback)
- `internal/config/model.go`, `schema/magelift.schema.json`, `docs/configuration.md` — DIY state fields

## Decisions Made

- **SSE mode for OVH/SCW:** `EncryptionAES256` (unit-proven). `EncryptionNone` kept as Floci fallback only if SSE headers are rejected — documented in Floci test, not selected as default.
- **Endpoint gate:** reuse `aws/endpoint.Parse` loopback-only for overrides (T-06-01); production OVH/SCW native endpoints deferred beyond Phase 6 offline scope.
- **OVH default S3 region:** `us-east-1` when `stateRegion` unset (GRA* is not an AWS region code); Scaleway defaults to Identity.Region.

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written for deliverables.

### Process notes

**1. [Rule 3 - Blocking] TDD RED was compile-fail, not a separate commit**
- **Found during:** Task 2
- **Issue:** Failing tests referenced undefined `State` / helpers before implementation existed
- **Fix:** Implemented GREEN immediately after RED compile evidence; single feat commit for Task 2
- **Files modified:** ovh/scaleway state packages
- **Verification:** unit tests pass
- **Committed in:** `449ad85`

**2. [Rule 2 - Critical] Added NewAWSArchiveWithEndpoint**
- **Found during:** Task 1/2
- **Issue:** Archive constructors needed the same endpoint override as Manager for OVH/SCW Backup/Restore
- **Fix:** Mirror `NewAWSWithEndpoint` for archives
- **Files modified:** `internal/cloud/aws/state/archive.go`
- **Verification:** AES256 archive unit test
- **Committed in:** `e90d629` / used by `449ad85`

## Encryption mode used for offline proof

**EncryptionAES256** (unit fake S3API asserts `ServerSideEncryption=AES256`). Floci live path not run this session (`MAGELIFT_FLOCI` unset); test falls back to EncryptionNone only if Floci rejects SSE.

## Threat Flags

None — no new trust-boundary surface beyond plan threat model (endpoint.Parse retained; no ARN/body logging).

## Self-Check: PASSED

- FOUND: `internal/cloud/aws/state/encryption.go`
- FOUND: `internal/cloud/ovh/stack/state.go`
- FOUND: `internal/cloud/scaleway/stack/state.go`
- FOUND: `tests/floci/state_aes256_test.go`
- FOUND: commit `e90d629`
- FOUND: commit `449ad85`
