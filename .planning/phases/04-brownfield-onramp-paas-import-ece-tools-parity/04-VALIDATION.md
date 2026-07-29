---
phase: 4
slug: brownfield-onramp-paas-import-ece-tools-parity
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-29
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Seeded from RESEARCH § Validation Architecture + plan `<automated>` commands.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + race (`make test`); PHPUnit via `make php-test` |
| **Config file** | `build/phpunit.xml`; Go packages self-contained |
| **Quick run command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ -count=1` |
| **Full suite command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ ./internal/build/... -count=1` then `make php-test` |
| **Estimated runtime** | ~30–120s focused Go; `make php-test` often >30s (wave/phase gate) |

---

## Sampling Rate

- **After every task commit:** Focused `go test` on touched packages (`GOMAXPROCS=1 GOFLAGS=-p=1`)
- **After every plan wave:** Importer+cli+build Go tests; `make php-test` when PHP touched
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** Prefer <30s for task commits; full php-test at wave/phase gate

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | IMPORT-01 | — | Synthetic ACC only | integration | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ -count=1 -run 'Acc\|FromAcc\|MapACC'` | ❌ W0 | ⬜ pending |
| 04-01-02 | 01 | 1 | IMPORT-05 | — | Reject foreign schema as deploy `--config` | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ ./internal/config/ -count=1 -run 'Foreign\|MagentoApp\|PlatformApp\|schemaVersion'` | ❌ W0 | ⬜ pending |
| 04-02-01 | 02 | 2 | IMPORT-01/02 | — | Checkpoint: lock `--from-acc`/`--from-upsun` (D-03) | decision | human gate | N/A | ⬜ pending |
| 04-02-02 | 02 | 2 | IMPORT-01/02 | — | Checkpoint: lock `--config-out` (D-04) | decision | human gate | N/A | ⬜ pending |
| 04-02-03 | 02 | 2 | IMPORT-01/02 | T-04 overwrite | Refuse exists unless `--yes`; mutual exclusive flags | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1 -run 'Init\|FromAcc\|FromUpsun\|ConfigOut\|Yes\|Mutual'` | ❌ W0 | ⬜ pending |
| 04-03-01 | 03 | 3 | IMPORT-03 | — | Checkpoint: lock D-05 unmapped sidecar + non-zero | decision | human gate | N/A | ⬜ pending |
| 04-03-02 | 03 | 3 | IMPORT-01..04, ECE-04 | V5/V6 | Allowlist map; unmapped reported; no plaintext crypt | unit/integration | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ -count=1 -run 'Unmapped\|Upsun\|Allowlist\|Acc\|Import'` | ❌ W0 | ⬜ pending |
| 04-03-03 | 03 | 3 | IMPORT-03, D-02 | — | `application.cron` + soak skip when env unset | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/config/ -count=1 -run 'Cron\|Soak\|Validate\|Schema\|Allowlist'` | ❌ W0 | ⬜ pending |
| 04-04-01 | 04 | 1 | ECE-03 | — | strategy/threads reach SCD argv | go+php | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/build/plan/ ./internal/build/runner/ -count=1 -run 'Static\|Strategy\|Thread' && make php-test` | ❌ W0 | ⬜ pending |
| 04-04-02 | 04 | 1 | ECE-03 | — | Schema keys for strategy/threads | unit | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ ./internal/build/plan/ -count=1 -run 'Static\|Schema\|Build'` | ❌ W0 | ⬜ pending |
| 04-05-01 | 05 | 2 | ECE-02 | — | Clean-room m2-hotfixes apply | phpunit | `make php-test` | ❌ W0 | ⬜ pending |
| 04-05-02 | 05 | 2 | ECE-02 | — | QUALITY_PATCHES intentional-gap stub | docs | `test -f docs/ece-parity.md && grep -E 'm2-hotfixes\|QUALITY_PATCHES' docs/ece-parity.md` | ❌ W0 | ⬜ pending |
| 04-06-01 | 06 | 4 | ECE-01, ECE-04 | T-04-12 | No blank status cells; closed ↔ shipped code | docs | python assert `not bad` closed/intentional-gap rows | ❌ W0 | ⬜ pending |
| 04-06-02 | 06 | 4 | IMPORT-06 | — | No vendored sibling trees; migration docs shipped | smoke | `bash scripts/check-clean-room.sh && grep … from-acc` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `testdata/fixtures/acc/` and `testdata/fixtures/upsun/` — supported + unmapped variants
- [ ] `internal/paasimport/` package + table tests
- [ ] CLI tests for `--from-acc`/`--from-upsun`, refuse/`--yes`, `--config-out`, unmapped exit
- [ ] Foreign-schema rejection test
- [ ] PHPUnit cases for patch apply + SCD `-s`/`-j`
- [ ] `scripts/check-clean-room.sh` + `docs/ece-parity.md` completeness assert

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Optional live soak fixtures | IMPORT-01/02 D-02 | Needs operator ACC/Upsun exports | Set `MAGELIFT_IMPORT_FIXTURE_*` and re-run soak tests; skip when unset |
| Checkpoint decisions D-03/D-04/D-05 | D-03..D-05 | One-way product doors | Confirm locked options in plan checkpoints during execute |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies / checkpoint gates
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency acceptable (focused Go <30s; php-test at wave gate)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
