---
phase: 01-publishable-baseline-honest-fallbacks
plan: 06
subsystem: testing
tags: [ovh, scaleway, depends-on, subnet-carve, quality-05, quality-08, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Lint coverage guard + offline CI deferral (01-02); AWS/GCP carve boundary pattern (01-05)
provides:
  - OVH subnetCIDR index boundary tests (0 / 15 / 16 / negative + sixteen-zone accept)
  - First test file in internal/cloud/ovh/runtime with RegisterRPC DependsOn capture
  - Scaleway single-private-network non-carving invariant
affects: [QUALITY-05, QUALITY-08, Phase 6 shared Kubernetes DependsOn harness]

tech-stack:
  added: []
  patterns:
    - "Capture DependsOn from pulumi.MockResourceArgs.RegisterRPC.GetDependencies(), never from resource registration order"
    - "Distinguish missing-resource (graph shape) vs missing-dependency (ordering regression) failure messages"
    - "Document deliberate non-applicability with an invariant test + doc comment when a provider has no carve"

key-files:
  created:
    - internal/cloud/ovh/runtime/runtime_test.go
  modified:
    - internal/cloud/ovh/network/network_test.go
    - internal/cloud/scaleway/network/network_test.go

key-decisions:
  - "Match dependency URNs by substring so mock project/stack names stay flexible"
  - "Scaleway QUALITY-05 clause closed as documented non-carving, not a false boundary test"
  - "Phase 6 ceiling: lift RegisterRPC DependsOn helper to shared kube layer for Kapsule/GKE"

patterns-established:
  - "Dependency-capture helper: mutex-guarded recordedResource{type,name,deps} from RegisterRPC"
  - "one()/assertDependsOn split missing-resource vs missing-substr failures"

requirements-completed: [QUALITY-05, QUALITY-08]

coverage:
  - id: D1
    description: OVH subnet carve asserted at index 0, 15, 16, negative, and both sides of the 16-zone /16 capacity boundary
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/ovh/network/ -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: OVH MKS node-pool DependsOn ordering asserted from RegisterRPC dependencies (provider, web, service, cron)
    requirement: QUALITY-08
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/ovh/runtime/ -count=1 -run TestMKSNodePoolDependencyOrdering"
        status: pass
    human_judgment: false
  - id: D3
    description: Scaleway creates exactly one private network with verbatim CIDR for any zone count
    requirement: QUALITY-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/scaleway/network/ -count=1 -run TestNewKeepsSinglePrivateNetworkRange"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 06: OVH/Scaleway Regression Guards Summary

**Both fixed OVH bugs now have regression tests (runtime package had none before), and Scaleway's deliberate non-carving is an explicit invariant instead of a silent gap.**

## Dependency-capture helper (Phase 6 reuse)

Shape used in `internal/cloud/ovh/runtime/runtime_test.go`:

1. `recordedResource{typeToken, name, dependencies []string}` — deps copied from `args.RegisterRPC.GetDependencies()` when non-nil.
2. Mutex-guarded `resources` slice on the package mock (`NewResource` append only).
3. `one(t, typeToken, nameSuffix)` — fail with "graph shape changed" if missing.
4. `assertDependsOn(t, rec, substrings...)` — fail with "ordering regressed" + deps dump if a substring is absent.

Do not assert on registration-slice order; the Go SDK registers concurrently.

## Performance

- **Duration:** ~5 min
- **Started:** 2026-07-28T11:51:42Z
- **Completed:** 2026-07-28T11:56:13Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- OVH `subnetCIDR` index cap covered at min/max/max+1 plus negative and sixteen-zone accept side
- First `ovh/runtime` test file guards pool→workload DependsOn via RegisterRPC
- Scaleway single-PN / verbatim-CIDR model documented by test across one and three zones

## Task Commits

1. **Task 1: Assert the OVH carve at its exact index boundaries** - `68e7538` (test)
2. **Task 2: Assert the MKS node-pool dependency ordering** - `4c1d49a` (test)
3. **Task 3: Make Scaleway's single-range model explicit** - `94139b6` (test)

**Plan metadata:** `40ec8b7` (docs: complete plan)

## Files Created/Modified

- `internal/cloud/ovh/network/network_test.go` — index boundary + sixteen-zone accept tests
- `internal/cloud/ovh/runtime/runtime_test.go` — new; RegisterRPC DependsOn capture + Phase 6 ceiling
- `internal/cloud/scaleway/network/network_test.go` — single-PN invariant across zone counts

## Decisions Made

- Substring URN matching over exact URNs (stable under mock project/stack rename)
- Scaleway closes QUALITY-05 as non-applicability with invariant + doc comment, not a fake carve boundary
- Marker comment names Phase 6 shared-layer lift for Kapsule/GKE

## Deviations from Plan

None - plan executed as written (test-only; production already correct). Task 2 deliberate regression (temporarily omit pool from `k8sOpts`) was observed failing then restored before commit; no production change shipped.

## Threat Flags

None — no new network endpoints, auth paths, or trust-boundary schema changes.

## Self-Check: PASSED

- FOUND: `internal/cloud/ovh/network/network_test.go`
- FOUND: `internal/cloud/ovh/runtime/runtime_test.go`
- FOUND: `internal/cloud/scaleway/network/network_test.go`
- FOUND: commits `68e7538`, `4c1d49a`, `94139b6`
- VERIFY: all three packages pass `go test -race`
