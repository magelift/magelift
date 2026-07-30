---
phase: 07-gcp-certification
plan: 03
subsystem: testing
tags: [gcp, acceptance, harness, checkpoint, evidence, wif, composer, day2]

requires:
  - phase: 07-gcp-certification
    provides: WIF Ensure + Composer SM (07-01), kube dumpimport runner (07-04), Cloudflare DNS script (07-05)
  - phase: 03-credit-efficient-acceptance-harness-evidence-tiering
    provides: shared lib-checkpoint / lib-evidence + dry-run harness shape
provides:
  - Expanded cells-gcp-preview.txt create-once catalog (WIF, Composer SM, day2, deploy, dump, cost, dns)
  - live_cell_loop in gcp-acceptance-local.sh with append_row + checkpoint resume
  - Offline dry-run green over full catalog (zero Pulumi create)
affects:
  - 07-06 live paid create-once pass
  - 07-07 certification docs / SC1–SC5 evidence

tech-stack:
  added: []
  patterns:
    - AWS-shaped live_cell_loop with GCP cell dispatch (never recreate between cells)
    - bootstrap:wif requires Act/STS proof — Ensure-only refused
    - GCP-scoped .magelift/gcp-matrix checkpoint + six-column matrix-results.md

key-files:
  created: []
  modified:
    - scripts/acceptance/cells-gcp-preview.txt
    - scripts/gcp-acceptance-local.sh
    - tests/acceptance/gcp_harness_shape_test.sh
    - docs/gcp-acceptance.md
    - scripts/acceptance/lib-checkpoint.sh
    - scripts/acceptance/lib-evidence.sh

key-decisions:
  - "Explicit composer:sm-write/sm-read cells for SC2 (not folded into day2:secrets alone)"
  - "bootstrap:wif refuses MAGELIFT_GCP_WIF_ENSURE_ONLY; needs Act log/flag or gcloud impersonation exchange"
  - "Placeholder DIGEST → create-once --infra-only; Magento cells require pullable digest"
  - "Offline dry-run only in 07-03 — no live up"

patterns-established:
  - "Pattern: gcp_acceptance_paths() pins checkpoint/evidence under .magelift/gcp-matrix/"
  - "Pattern: run_gcp_cell case dispatch references 07-01/04/05 tools (bootstrap, dumpimport kube env, cutover-dns script)"

requirements-completed: [GCP-03, GCP-04, GCP-05]

coverage:
  - id: D1
    description: Expanded GCP cell catalog with day2/deploy/dump/cost/dns (+ WIF + Composer SM)
    requirement: GCP-03
    verification:
      - kind: other
        ref: "rg day2:|deploy:candidate|migrate:dump|cost:estimate|cutover:dns scripts/acceptance/cells-gcp-preview.txt"
        status: pass
    human_judgment: false
  - id: D2
    description: Dry-run live_cell_loop shape with append_row and zero Pulumi create
    requirement: GCP-04
    verification:
      - kind: integration
        ref: "MAGELIFT_ACCEPTANCE_DRY_RUN=1 bash tests/acceptance/gcp_harness_shape_test.sh"
        status: pass
    human_judgment: false
  - id: D3
    description: Checkpoint resume + six-column evidence contract documented for SC1–SC5 certification
    requirement: GCP-05
    verification:
      - kind: integration
        ref: "gcp_harness_shape_test.sh resume skip + rg checkpoint|append_row|matrix-results"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-30
status: complete
---

# Phase 07 Plan 03: GCP Harness Cell Catalog Summary

**Expanded GCP acceptance catalog with live_cell_loop (create-once → cell updates → append_row) proven green offline with Act/STS WIF gate and Composer SM cells.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-30T11:48:10Z
- **Completed:** 2026-07-30T11:51:50Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Catalog lists ordered resume-friendly cells: `bootstrap:wif`, Composer SM write/read, day2 secrets/state/logs/exec/health, `deploy:candidate`, `migrate:dump`, `cost:estimate`, `cutover:dns`
- `live_cell_loop` wired on `MAGELIFT_GCP_ACCEPTANCE=1 up` after create-once; dry-run loops all cells with `append_row` and `created=0`
- Shape test proves checkpoint resume skip + EXIT `destroy` → `force_clean_orphans` → `assert_clean` symbols; DIGEST must be pullable for Magento cells

## Task Commits

1. **Task 1: End-to-end dry-run cell loop with full catalog** - `ad00269` (feat)
2. **Task 2: Checkpoint resume + evidence column contract** - `ab7ce6c` (docs)

**Plan metadata:** `e673fd8` (docs: complete plan)

## Files Created/Modified

- `scripts/acceptance/cells-gcp-preview.txt` — full Phase 7 cell order
- `scripts/gcp-acceptance-local.sh` — `live_cell_loop` / `run_gcp_cell` / WIF Act-STS proof / serial build
- `tests/acceptance/gcp_harness_shape_test.sh` — expanded catalog + resume assertions
- `docs/gcp-acceptance.md` — evidence/resume/DIGEST/WIF contract
- `scripts/acceptance/lib-checkpoint.sh` — resume + GCP path notes
- `scripts/acceptance/lib-evidence.sh` — append_row-only certification note

## Decisions Made

- Separate `composer:sm-write` / `composer:sm-read` cells so 07-06 SC2 evidence is not optionalized by catalog
- `bootstrap:wif` fails closed without Act (`MAGELIFT_GCP_WIF_ACT_LOG` / `_PROOF`) or gcloud impersonation STS exchange
- Placeholder digest keeps create-once on `--infra-only`; Magento day2/deploy cells refuse placeholder

## Deviations from Plan

None - plan executed exactly as written (offline dry-run only; no live create).

## Issues Encountered

None blocking. Known follow-on for 07-06: `env import-dump` still needs CLI to honor `MAGELIFT_DUMPIMPORT_RUNNER=kube` for private Cloud SQL (07-04 runner exists; harness already exports the env). Documented in `docs/gcp-acceptance.md` Known gaps.

## Known Stubs

None that prevent the plan goal (dry-run catalog + live loop shape). Live Magento/WIF/DNS behaviors remain for 07-06 paid pass.

## User Setup Required

None for this offline plan. Live pass (07-06) still needs pullable DIGEST, ADC, Cloudflare Zone.DNS Edit token, and Act/STS WIF proof.

## Next Phase Readiness

- Harness ready for single create-once live pass (07-06)
- 07-02 cost estimator / remaining incomplete plans can proceed in parallel where independent

## Self-Check: PASSED

- FOUND: scripts/acceptance/cells-gcp-preview.txt
- FOUND: scripts/gcp-acceptance-local.sh
- FOUND: tests/acceptance/gcp_harness_shape_test.sh
- FOUND: docs/gcp-acceptance.md
- FOUND: scripts/acceptance/lib-checkpoint.sh
- FOUND: scripts/acceptance/lib-evidence.sh
- FOUND: ad00269
- FOUND: ab7ce6c

---
*Phase: 07-gcp-certification*
*Completed: 2026-07-30*
