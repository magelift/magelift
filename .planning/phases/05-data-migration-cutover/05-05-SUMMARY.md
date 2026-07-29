---
phase: 05-data-migration-cutover
plan: 05
subsystem: migration
tags: [media-sync, s3, floci, cobra, migrate-03]

requires:
  - phase: 05-data-migration-cutover
    provides: D-05 locked media-sync command (no seedMedia auto); Floci S3 contract from prior storage tests
provides:
  - magelift env media-sync --source with merge upload + listing-diff verification
  - internal/mediasync library with injectable S3 client
  - Offline unit + Floci empty listing-diff proof (SC4 / MIGRATE-03)
affects: [05-06 cutover runbook, seedMedia follow-on]

tech-stack:
  added: []
  patterns:
    - "mediasync library + injectable S3 API; CLI resolves mediaBucket from stack outputs"
    - "Merge default (no remote deletes); pub/media/ key strip or source-as-media-root"
    - "Floci MAGELIFT_FLOCI=1 skip gate mirroring storage_test.go"

key-files:
  created:
    - internal/mediasync/sync.go
    - internal/mediasync/sync_test.go
    - internal/cli/env_media_sync_test.go
    - testdata/fixtures/migrate/media/catalog/product/fixture.txt
    - tests/floci/media_sync_test.go
  modified:
    - internal/cli/env.go
    - internal/cli/root.go
    - internal/platform/outputs.go
    - internal/cloud/aws/stack/component.go
    - internal/cloud/aws/stack/module.go
    - internal/cloud/aws/stack/component_test.go

key-decisions:
  - "Default merge: PutObject only; missing-key drift fails; remote extras allowed"
  - "Export AWS stack mediaBucket (parity with GCP) so CLI can resolve the target bucket"
  - "No seedMedia YAML or auto-after-deploy in this plan (D-05 follow-on)"

patterns-established:
  - "Pattern 4 media sync: walk → map keys → PutObject → ListObjectsV2 → DiffKeys"
  - "Seekable *os.File bodies for Floci/AWS SDK v2 checksum middleware"

requirements-completed: [MIGRATE-03]

coverage:
  - id: D1
    description: env media-sync --source uploads fixture media and listing missing-key diff is empty
    requirement: MIGRATE-03
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/mediasync/ ./internal/cli/ -count=1 -run 'MediaSync|ListingDiff|mediasync'"
        status: pass
    human_judgment: false
  - id: D2
    description: Floci offline proof of media-sync listing diff empty
    requirement: MIGRATE-03
    verification:
      - kind: integration
        ref: "MAGELIFT_FLOCI=1 make floci-test (TestMediaSyncListingDiffAgainstFloci)"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-29
status: complete
---

# Phase 5 Plan 05: Media Sync Summary

**`magelift env media-sync --source` uploads a local media tree to the env media bucket with merge semantics and empty listing-diff proof on unit fake + Floci (SC4 / MIGRATE-03).**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-29T16:18:51Z
- **Completed:** 2026-07-29T16:22:20Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments

- Shipped `internal/mediasync` with confined `--source`, `pub/media/` key mapping, merge PutObject, and listing DiffKeys
- Wired `env media-sync` cobra command; help documents merge + key mapping; injectable sync for CLI tests
- Exported AWS `mediaBucket` stack output (GCP already had it) for bucket resolution
- Floci `TestMediaSyncListingDiffAgainstFloci` passed under `MAGELIFT_FLOCI=1 make floci-test`
- No `seedMedia` auto-after-deploy (deferred per D-05)

## Task Commits

1. **Task 1: End-to-end media-sync --source → empty listing diff** - `fd09c21` (feat)
2. **Task 2: Floci media-sync listing-diff proof** - `c742f03` (test)

## Files Created/Modified

- `internal/mediasync/sync.go` — Sync library + DiffKeys
- `internal/mediasync/sync_test.go` — fake S3 listing-diff / merge / path escape tests
- `internal/cli/env.go` — `envMediaSyncCommand`, bucket resolve, S3 client via `MAGELIFT_AWS_ENDPOINT_URL`
- `internal/cli/root.go` — register `media-sync` under env group
- `internal/cli/env_media_sync_test.go` — CLI inject + help docs tests
- `testdata/fixtures/migrate/media/catalog/product/fixture.txt` — synthetic fixture
- `tests/floci/media_sync_test.go` — Floci integration proof
- `internal/platform/outputs.go` — `OutputMediaBucket`
- `internal/cloud/aws/stack/{component,module,component_test}.go` — export `mediaBucket`

## Decisions Made

- Merge-only verification fails on missing source keys; extras remain (merge honesty)
- AWS needed `mediaBucket` exported for the CLI seam RESEARCH assumed already existed
- Floci proof calls `mediasync.Sync` directly (same client shape as storage_test)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] AWS stack lacked mediaBucket output**
- **Found during:** Task 1 (bucket resolution)
- **Issue:** RESEARCH/GCP expose `mediaBucket`; AWS Outputs only had `mediaURL`, so CLI could not resolve the sync target
- **Fix:** Export `c.Storage.BucketName` as `mediaBucket` on AWS stack Outputs + OutputKeys + constant
- **Files modified:** `internal/cloud/aws/stack/component.go`, `module.go`, `component_test.go`, `internal/platform/outputs.go`
- **Verification:** unit media-sync tests green; stack component_test expects `mediaBucket`
- **Committed in:** `fd09c21` (part of task 1)

## Auth Gates

None.

## Known Stubs

None.

## Threat Flags

None beyond plan register (T-05-12 mitigated via confineSource / walk Rel escape checks).

## Self-Check: PASSED

- FOUND: `internal/mediasync/sync.go`
- FOUND: `internal/cli/env_media_sync_test.go`
- FOUND: `tests/floci/media_sync_test.go`
- FOUND: `testdata/fixtures/migrate/media/catalog/product/fixture.txt`
- FOUND: `fd09c21`
- FOUND: `c742f03`
