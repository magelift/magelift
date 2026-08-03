---
phase: 07-gcp-certification
plan: 06
subsystem: infra
tags: [gcp, gke-autopilot, acceptance, wif, migrate-04]

requires:
  - phase: 07-gcp-certification
    provides: offline harness 07-01..05 + Cloudflare DNS script
provides:
  - Live create-once matrix PASS rows for SC1–SC5 + migrate:dump + cutover:dns
  - Destroy + force_clean + assert_clean + DNS cleanup evidence
affects: [07-07 certification flip, multi-cloud ADR 0007]

tech-stack:
  added: []
  patterns: [KEEP on cell FAIL, infra-only create-once, decrypted Outputs for day-2]

key-files:
  created:
    - .planning/phases/07-gcp-certification/scratch/07-06-live-pass.md
  modified:
    - .magelift/gcp-matrix/matrix-results.md
    - .magelift/gcp-matrix/acceptance-checkpoint.json
    - internal/config/config.go
    - scripts/gcp-acceptance-local.sh
    - internal/cloud/gcp/** (day-2 BindOutputs / dump / DNS cells)

key-decisions:
  - "Create-once uses --infra-only; Magento via deploy:candidate cell"
  - "KEEP=true on cell FAIL to avoid destroy mid-matrix; final EXIT destroy + force_clean"
  - "Certification evidence is harness append_row PASS rows (FAIL+resume allowed)"

patterns-established:
  - "GCP live pass: ADC + Cloudflare token + destroy-when-done before spend"
  - "force_clean + PSA soak when Pulumi destroy stalls on producer services"

requirements-completed: [GCP-01, GCP-02, GCP-03, GCP-04, GCP-05, MIGRATE-04]

coverage:
  - id: D1
    description: "Live WIF bootstrap + Composer SM write/read PASS (SC1–SC2)"
    requirement: GCP-01
    verification:
      - kind: e2e
        ref: ".magelift/gcp-matrix/matrix-results.md bootstrap:wif|composer:sm-*"
        status: pass
    human_judgment: false
  - id: D2
    description: "Day-2 secrets/state/logs/exec/health PASS (SC3)"
    requirement: GCP-03
    verification:
      - kind: e2e
        ref: ".magelift/gcp-matrix/matrix-results.md day2:*"
        status: pass
    human_judgment: false
  - id: D3
    description: "deploy:candidate + cost:estimate PASS (SC4–SC5)"
    requirement: GCP-04
    verification:
      - kind: e2e
        ref: ".magelift/gcp-matrix/matrix-results.md deploy:candidate|cost:estimate"
        status: pass
    human_judgment: false
  - id: D4
    description: "migrate:dump + cutover:dns live PASS; DNS cleaned on teardown"
    requirement: MIGRATE-04
    verification:
      - kind: e2e
        ref: "scratch/07-06-live-pass.md + matrix migrate:dump|cutover:dns"
        status: pass
    human_judgment: false
  - id: D5
    description: "assert_clean ok + state bucket deleted + no mlgcpwt leftovers"
    requirement: GCP-05
    verification:
      - kind: other
        ref: "scratch/07-06-live-teardown.log TEARDOWN_DONE"
        status: pass
    human_judgment: false

duration: ~6h
completed: 2026-08-02
status: complete
---

# Phase 7 Plan 06: Live GCP Create-Once Pass Summary

**Single paid GKE Autopilot create-once on `digital-lab-341608` recorded harness PASS for all required cells; teardown reached `assert_clean ok`, DNS cleanup, and empty account.**

## Performance

- **Duration:** multi-hour live pass with mid-matrix resumes (KEEP)
- **Completed:** 2026-08-02T13:57:38Z (`TEARDOWN_DONE`)
- **Tasks:** prereqs + create-once cells + EXIT destroy/force_clean
- **Files modified:** harness evidence + live-pass scratch + runtime fixes during pass

## Accomplishments

- All 12 catalog cells ended PASS (WIF, Composer SM, day2×5, deploy, dump, cost, DNS)
- MIGRATE-04 live half evidenced: managed dump cell + Cloudflare cutover + `--cleanup`
- Mandatory cleanup: force_clean, PSA soak, assert_clean, DNS delete, Pulumi stack rm, GCS state bucket deleted

## Evidence

- `.magelift/gcp-matrix/matrix-results.md`
- `.planning/phases/07-gcp-certification/scratch/07-06-live-pass.md`
- `.planning/phases/07-gcp-certification/scratch/07-06-live-teardown.log`

## Next

Plan 07-07: flip certified docs + REQUIREMENTS GCP-06 / MIGRATE-04 using this evidence only.
