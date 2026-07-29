---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 02
subsystem: cli
tags: [paas-import, init, from-acc, from-upsun, config-out, cobra]

requires:
  - phase: 04-brownfield-onramp-paas-import-ece-tools-parity
    provides: ACC MapACC tracer and init --from-acc refuse-if-exists baseline (04-01)
provides:
  - Mutually exclusive init --from-acc / --from-upsun (D-03)
  - Refuse existing write path unless persistent --yes (D-04)
  - init-local --config-out PATH side-file write avoiding root -o collision
  - Thin paasimport.MapUpsun stub for CLI plumbing
affects:
  - 04-03 Upsun mapper + unmapped sidecar
  - docs/migrating-from-paas.md operator wording

tech-stack:
  added: []
  patterns:
    - "init-local --config-out for side-file; never local --output (collides with persistent format -o)"
    - "Persistent --yes gates overwrite of chosen write path (config or config-out)"
    - "Mutual exclusion of --from-acc/--from-upsun before any filesystem write"

key-files:
  created:
    - internal/paasimport/upsun.go
  modified:
    - internal/cli/root.go
    - internal/cli/init_import_test.go
    - docs/cli-reference.md

key-decisions:
  - "Checkpoint D-03: Ship --from-acc and --from-upsun on init (d03-literal)"
  - "Checkpoint D-04: Use init-local --config-out PATH (config-out), not --write or root -o"
  - "MapUpsun thin stub emits Load-valid schemaVersion-1 YAML until 04-03"

patterns-established:
  - "Pattern: writePath = config-out if set else --config; refuse/overwrite applies to writePath only"
  - "Pattern: bare init --yes rewrites starterConfig at write path"

requirements-completed: [IMPORT-01, IMPORT-02]

coverage:
  - id: D1
    description: Mutual exclusion of --from-acc and --from-upsun yields exit 2
    requirement: IMPORT-01
    verification:
      - kind: unit
        ref: "internal/cli/init_import_test.go#TestInitFromAccAndFromUpsunMutualExclusion"
        status: pass
    human_judgment: false
  - id: D2
    description: Existing config refuses without --yes; overwrites with --yes
    requirement: IMPORT-01
    verification:
      - kind: unit
        ref: "internal/cli/init_import_test.go#TestInitFromAccOverwriteWithYes"
        status: pass
    human_judgment: false
  - id: D3
    description: --config-out writes side file and leaves default magelift.yaml untouched
    requirement: IMPORT-01
    verification:
      - kind: unit
        ref: "internal/cli/init_import_test.go#TestInitConfigOutWritesSideFileLeavesDefault"
        status: pass
    human_judgment: false
  - id: D4
    description: --from-upsun accepted and writes Load-valid YAML via MapUpsun stub
    requirement: IMPORT-02
    verification:
      - kind: unit
        ref: "internal/cli/init_import_test.go#TestInitFromUpsunAccepted"
        status: pass
    human_judgment: false

duration: 2min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 02: Init import CLI contract Summary

**Published init CLI locks D-03 `--from-acc`/`--from-upsun` mutual exclusion and D-04 refuse/`--yes` plus `--config-out` side-file write (no root `-o` collision).**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-07-29T15:29:05Z
- **Completed:** 2026-07-29T15:30:43Z
- **Tasks:** 3/3 (2 auto-selected checkpoints + 1 TDD)
- **Files modified:** 4

## Accomplishments

- Locked execute-time checkpoints: D-03 `d03-literal`, D-04 `config-out`
- Extended `init` with `--from-upsun`, `--config-out`, mutual exclusion, and `--yes` overwrite of the chosen write path
- Regenerated `docs/cli-reference.md` listing locked flag names
- Thin `paasimport.MapUpsun` stub so Upsun flag is callable offline until 04-03 mapping

## Checkpoint Resolutions

| Gate | Selection | Rationale |
|------|-----------|-----------|
| D-03 flag pair | `d03-literal` — Ship `--from-acc` and `--from-upsun` on init | Locked CONTEXT / IMPORT-01/02; avoids `promote --from` collision |
| D-04 side-file name | `config-out` — init-local `--config-out PATH` | Research recommendation; avoids persistent `--output`/`-o` format collision |

## Task Commits

1. **Tasks 1–2: Checkpoint decisions (D-03, D-04)** — auto-selected per orchestrator (no commit; recorded above)
2. **Task 3: Init CLI contract** (TDD)
   - RED - `42e46b7` (test)
   - GREEN - `332568a` (feat)

**Plan metadata:** *(pending final docs commit)*

## Files Created/Modified

- `internal/cli/root.go` — init flags, mutual exclusion, refuse/`--yes`, `--config-out` write path
- `internal/cli/init_import_test.go` — exclusion, overwrite, config-out, Upsun acceptance, bare `--yes`
- `internal/paasimport/upsun.go` — thin MapUpsun stub
- `docs/cli-reference.md` — generated help for locked flags

## Decisions Made

- Auto-selected D-03 `d03-literal` and D-04 `config-out` (user locked via discuss --auto)
- Reuse persistent `--yes` only — no `--force` / overwrite alias
- MapUpsun stub returns minimal Load-valid YAML (not hard error) so CLI plumbing tests green before 04-03

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Corrected STATE after advance-plan miscount**
- **Found during:** State updates after SUMMARY
- **Issue:** `state.advance-plan` treated plan 4/4 as last plan and set `status: verifying` / "Phase complete" while 04-03/05/06 remain
- **Fix:** Restored `status: executing` and accurate remaining-plan position (same pattern as 04-04 correction)
- **Files modified:** `.planning/STATE.md`
- **Verification:** ROADMAP still shows 3/6 Phase 4; incomplete plans 04-03, 04-05, 04-06 on disk
- **Committed in:** docs metadata commit

**Total deviations:** 1 auto-fixed (Rule 3)
**Impact on plan:** No code impact; planning-state hygiene only.

## Known Stubs

| File | Line | Stub | Reason |
|------|------|------|--------|
| `internal/paasimport/upsun.go` | MapUpsun | Emits `baseDocument("upsun-import", "8.3")` without parsing Platform.sh / Upsun trees | Planned thin adapter until 04-03 full mapper |

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CLI surface ready for 04-03 to replace MapUpsun stub + add unmapped sidecar (D-05)
- No blockers for remaining Phase 4 plans

## Self-Check: PASSED

- FOUND: `internal/cli/root.go`, `internal/cli/init_import_test.go`, `internal/paasimport/upsun.go`, `docs/cli-reference.md`
- FOUND: commits `42e46b7`, `332568a`
- VERIFY: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1 -run 'Init|FromAcc|FromUpsun|ConfigOut|Yes|Mutual'` → ok

---
*Phase: 04-brownfield-onramp-paas-import-ece-tools-parity*
*Completed: 2026-07-29*
