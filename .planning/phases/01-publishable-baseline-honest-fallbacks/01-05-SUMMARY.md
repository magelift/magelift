---
phase: 01-publishable-baseline-honest-fallbacks
plan: 05
subsystem: networking
tags: [cidr, subnet-carve, quality-05, aws, gcp, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Lint coverage guard + offline CI deferral (01-02)
provides:
  - AWS subnet carve capacity check (bits+4 → 16 slots; max 5 zones)
  - GCP subnet index cap (/20 → 8 legal indices 0–7) and first package tests
affects: [QUALITY-05, 01-06 OVH/Scaleway carve tests]

tech-stack:
  added: []
  patterns:
    - "Express carve capacity from prefix arithmetic, never hard-coded slot counts"
    - "Reject carve overflow before Pulumi registration; name available and demanded limits"

key-files:
  created:
    - internal/cloud/gcp/network/network_test.go
  modified:
    - internal/cloud/aws/network/network.go
    - internal/cloud/aws/network/network_test.go
    - internal/cloud/gcp/network/network.go

key-decisions:
  - "AWS: run validateCarveCapacity before preset zone-count so 6+ zones fail with isolation error"
  - "GCP: caller names zone capacity; helper names subnet index — both kept"
  - "No shared cross-provider carve helper (ADR 0002/0008 adapter isolation)"

patterns-established:
  - "Boundary tests at min, max, max+1 with explicit parent-prefix containment on max"
  - "GCP network package now has reject-before-register mocks like AWS/OVH"

requirements-completed: [QUALITY-05]

coverage:
  - id: D1
    description: AWS rejects zone counts that overflow bits+4 carve slots before registration
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/network/ -run SubnetCarveBoundaries -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: GCP rejects subnet index and over-long zone lists beyond parent capacity before registration
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/gcp/network/ -count=1"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 05: AWS/GCP Subnet Carve Caps Summary

**AWS and GCP now reject subnet carves that would land outside the parent prefix, with min/max/max+1 boundary tests (GCP package had none before).**

## Carve arithmetic (traceable)

| Provider | Carve rule | Slots / indices | Max legal | Max+1 |
|----------|------------|-----------------|-----------|-------|
| AWS | `prefix.Bits()+4` blocks; 3 per zone | **16** blocks | **5** zones (15 blocks) | **6** zones (18 demanded) |
| GCP | two /24s per index at `index*2` | **/20 → 16 /24s → 8** indices | indices **0–7** | index **8** / 9 zones |

## Performance

- **Duration:** ~5 min
- **Started:** 2026-07-28T11:45:20Z
- **Completed:** 2026-07-28T11:50:43Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Added `validateCarveCapacity` on AWS; capacity checked before preset zone counts so overflow is an isolation error
- Added GCP `subnetIndexCapacity`, index guard in `subnetCIDRs`, and zone-list rejection before `RegisterComponentResourceV2`
- Created `internal/cloud/gcp/network/network_test.go` — first tests for the package
- Both packages pass `go test -race` including AWS pre-existing snapshot tests

## Task Commits

1. **Task 1: Cap the AWS subnet carve and test its boundaries** - `beda109` (feat)
2. **Task 2: Cap the GCP subnet carve and give the package its first tests** - `ca0f31c` (test RED) + `c884e16` (feat GREEN)

**Plan metadata:** _(pending final docs commit)_

## Files Created/Modified

- `internal/cloud/aws/network/network.go` — `validateCarveCapacity`; capacity before preset zone check
- `internal/cloud/aws/network/network_test.go` — `TestSubnetCarveBoundaries` (min/max/max+1)
- `internal/cloud/gcp/network/network.go` — index cap + pre-register zone capacity
- `internal/cloud/gcp/network/network_test.go` — new boundary and reject-before-register tests

## Decisions Made

- Place AWS capacity check before preset zone-count so six zones surface the carve limit (otherwise dead behind exact zone match)
- Keep separate GCP caller vs helper error messages (zone capacity vs subnet index)
- Do not introduce a shared cross-provider carve helper

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Critical] AWS capacity check ordered before preset zone counts**
- **Found during:** Task 1
- **Issue:** Exact preset zone counts (2 or 3) made a six-zone overflow unreachable; max+1 could only fail with a preset-policy message
- **Fix:** Call `validateCarveCapacity` immediately after VPC CIDR parse and before the preset zone-count check
- **Files modified:** `internal/cloud/aws/network/network.go`
- **Verification:** `TestSubnetCarveBoundaries/max+1` asserts error names slot/demand counts and zero Pulumi resources
- **Committed in:** `beda109`

---

**Total deviations:** 1 auto-fixed (Rule 2)
**Impact on plan:** Required for QUALITY-05 max+1 / T-01-16 to be observable; no live accepted config changes.

## Issues Encountered

None

## Known Stubs

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 01-05 complete offline; next plan **01-06** (OVH/Scaleway QUALITY-05/08 carve tests)
- AWS+GCP carve caps closed; OVH already had a cap — 01-06 completes the per-provider matrix

## Self-Check: PASSED

- FOUND: `internal/cloud/aws/network/network.go` (`validateCarveCapacity`)
- FOUND: `internal/cloud/aws/network/network_test.go` (`TestSubnetCarveBoundaries`)
- FOUND: `internal/cloud/gcp/network/network.go` (`subnetIndexCapacity`)
- FOUND: `internal/cloud/gcp/network/network_test.go`
- FOUND: commits `beda109`, `ca0f31c`, `c884e16`

---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*
