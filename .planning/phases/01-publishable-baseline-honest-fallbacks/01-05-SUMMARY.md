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
  - GCP subnet index cap (two /24s per index; /20 → indices 0–7)
  - First tests for internal/cloud/gcp/network/
affects: [QUALITY-05, 01-06 OVH/Scaleway carve tests]

tech-stack:
  added: []
  patterns:
    - "Express carve caps from the same bit arithmetic as the carve, never hard-coded slot counts"
    - "Caller rejects over-long zone lists before Pulumi registration; helper also caps index"

key-files:
  created:
    - internal/cloud/gcp/network/network_test.go
  modified:
    - internal/cloud/aws/network/network.go
    - internal/cloud/aws/network/network_test.go
    - internal/cloud/gcp/network/network.go

key-decisions:
  - "AWS: 16 slots at bits+4, 3 blocks/zone → max 5 zones; reject at 6 naming available and demanded"
  - "GCP: /20 holds 16 /24s → 8 indices (0–7); caller names zone capacity, helper names index limit"
  - "AWS capacity check runs before preset zone-count so ≥6 zones surface the isolation error"

patterns-established:
  - "Per-provider carve caps stay local — no shared cross-adapter helper (ADR 0002/0008)"
  - "Boundary tests assert VPC/parent containment at max, not only string equality"

requirements-completed: [QUALITY-05]

coverage:
  - id: D1
    description: AWS rejects carve demand beyond bits+4 slots with min/max/max+1 tests
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/network/ -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: GCP rejects index/zone overflow before registration; package has first tests
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/gcp/network/ -count=1"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 05: AWS & GCP Subnet Carve Caps Summary

**AWS and GCP now reject subnet carves that would land outside the parent prefix, with min/max/max+1 boundary tests (GCP's first package tests).**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-28T11:46:23Z
- **Completed:** 2026-07-28T11:50:18Z
- **Tasks:** 2
- **Files modified:** 4

## Carve arithmetic (traceable)

| Provider | Carve rule | Computed capacity | Max legal | Max+1 |
|----------|------------|-------------------|-----------|-------|
| AWS | `bits+4` blocks; 3 blocks/zone (public/private/data) | `1 << 4` = **16 slots** | **5 zones** (15 blocks) | 6 zones → 18 demanded |
| GCP | two /24s per index at `index*2` | `/20` → `1<<(24-20)` = 16 /24s → **8 indices** | **index 7** (zones 0–7) | index 8 / 9 zones |

## Accomplishments

- AWS `validateCarveCapacity` rejects before `subnetCIDRs`; snapshot tests unchanged (live CIDRs byte-identical).
- GCP `subnetIndexCapacity` + caller zone-list check reject before any Pulumi registration; helper keeps index cap.
- New `internal/cloud/gcp/network/network_test.go` covers min, max, max+1, `/20` floor, and reject-before-register.

## Task Commits

1. **Task 1: Cap the AWS subnet carve and test its boundaries** - `beda109` (feat)
2. **Task 2: Cap the GCP subnet carve and give the package its first tests** - `ca0f31c` (test RED), `c884e16` (feat GREEN)

**Plan metadata:** (pending docs commit)

## Files Created/Modified

- `internal/cloud/aws/network/network.go` — `validateCarveCapacity`; capacity before preset zone policy
- `internal/cloud/aws/network/network_test.go` — `TestSubnetCarveBoundaries` (min/max/max+1 + Pulumi zero resources)
- `internal/cloud/gcp/network/network.go` — index cap + pre-register zone capacity
- `internal/cloud/gcp/network/network_test.go` — first package tests

## Decisions Made

- Caps derived from carve bit arithmetic (`bits+4` / `24-prefix.Bits()`), not hard-coded 16/8.
- No shared cross-provider carve helper — four providers, four designs.
- AWS capacity check ordered before preset AZ count so six-plus zones report isolation failure, not preset policy.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Critical] AWS capacity check reachable for reject-before-register**
- **Found during:** Task 1
- **Issue:** Preset policy requires exactly 2–3 AZs and ran before any carve check, so six zones never reached a capacity error message through `New`.
- **Fix:** Run CIDR + carve capacity validation before the preset zone-count check. Configs that already failed preset still fail; only ≥6 zones change message from preset to carve capacity.
- **Files modified:** `internal/cloud/aws/network/network.go`
- **Verification:** `TestSubnetCarveBoundaries/max+1` asserts message names slots/demand and zero Pulumi resources
- **Committed in:** `beda109`

**2. [Rule 2 - Critical] GCP `/20` and zone capacity before registration**
- **Found during:** Task 2
- **Issue:** `/20` width and index errors lived inside `subnetCIDRs` after `RegisterComponentResourceV2`, violating reject-before-register for over-long zone lists.
- **Fix:** Mirror `/20` message and zone-capacity check in `New` before any registration; keep helper index cap.
- **Files modified:** `internal/cloud/gcp/network/network.go`
- **Verification:** `TestNewRejectsOverlongZoneListBeforeRegistration`
- **Committed in:** `c884e16`

## Threat Flags

None — mitigations T-01-16, T-01-17, T-01-18, T-01-19 addressed; no new trust-boundary surface beyond planned validation.

## Known Stubs

None.

## Self-Check: PASSED

- Created/modified files present: aws+gcp `network.go`/`network_test.go`, SUMMARY
- Commits present: `beda109`, `ca0f31c`, `c884e16`
