---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
verified: 2026-07-29T15:49:28Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification: false
---

# Phase 4: Brownfield Onramp — PaaS Import & ece-tools Parity Verification Report

**Phase Goal:** A store already running on Adobe Commerce Cloud or Upsun can generate a reviewable `magelift.yaml` from its existing config and trust that MageLift's PHP build system does what `ece-tools` did for it.

**Verified:** 2026-07-29T15:49:28Z  
**Status:** passed  
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `magelift init --from-acc` / `--from-upsun` against fixtures write `magelift.yaml` that passes `config validate` / `config.Load` with zero hand edits | ✓ VERIFIED | `TestInitFromAccWritesLoadValidConfig` runs `config validate`; `TestInitFromUpsunAccepted`, `TestMapACCSupportedFixture`, `TestMapUpsunSupportedFixture`, `TestSupportedImportsValidate` all pass; fixtures under `testdata/fixtures/{acc,upsun}/supported/` |
| 2 | Unmappable keys produce durable report, refuse success (non-zero), with key coverage of app/services/routes/cron domains | ✓ VERIFIED | `TestInitFromAccUnmappedWritesSidecarAndExitsNonZero`, `TestMapACCUnmappedFixture` / Upsun variant assert named keys (`hooks.*`, `crons.shell-*`, residual env); Magento cron maps (`TestMapACCCronMapsMagentoOnly`); services/routes mapped via relationships/`domainFromRoutes` on supported path; CLI writes `magelift.unmapped.md` then `invalid()` |
| 3 | Foreign PaaS YAML rejected as `--config`; clean-room provenance + no vendored reference trees | ✓ VERIFIED | `TestForeignMagentoAppRejectedAsConfig` / `TestForeignPlatformAppRejectedAsConfig`; `config.Load` requires `schemaVersion: 1` (`internal/config/config.go`); deploy/validate share `options.load()` → `config.Load`; `bash scripts/check-clean-room.sh` exit 0; knowledge lesson + `docs/provenance.md` record Phase 4 clean-room import |
| 4 | `docs/ece-parity.md` every hook/env row `closed` or `intentional-gap` with reason — no blank status | ✓ VERIFIED | Exact 04-06 assert: `rows 35 bad 0`; `m2-hotfixes` present; QUALITY_PATCHES = `intentional-gap` |
| 5 | `make php-test` covers m2-hotfixes-style patches and SCD locales/themes/strategy/threads | ✓ VERIFIED | `make php-test` → PHPUnit **110 tests, 254 assertions OK**; `PatchApplierTest` apply/fail-loud; `LifecyclePlanTest` / `NativePreparationTest` assert `-s`/`-j` in SCD argv |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `testdata/fixtures/acc/{supported,unmapped}/` | Synthetic ACC config trees | ✓ VERIFIED | `.magento.app.yaml`, services, routes, env present |
| `testdata/fixtures/upsun/{supported,unmapped}/` | Synthetic Upsun/Platform trees | ✓ VERIFIED | `.platform.app.yaml` + sibling configs |
| `internal/paasimport/` | Shared mapper + allowlist + unmapped sidecar | ✓ VERIFIED | 9 Go files; MapACC/MapUpsun wired; tests green |
| `internal/cli/root.go` init flags | `--from-acc`, `--from-upsun`, `--config-out` | ✓ VERIFIED | Mutual exclusion, refuse exit 2, `--yes` overwrite, sidecar on unmapped |
| `build/src/Magento/PatchApplier.php` | Clean-room hotfix apply | ✓ VERIFIED | 208 lines; called from `NativePreparation` / `LifecyclePlan` |
| `build/src/Magento/LifecyclePlan.php` | SCD `-s`/`-j` emission | ✓ VERIFIED | strategy/threads → argv |
| `docs/ece-parity.md` | Complete matrix | ✓ VERIFIED | 35 data rows, no blanks |
| `scripts/check-clean-room.sh` | Vendoring gate | ✓ VERIFIED | Wired via `make check-clean-room` / `make verify` |
| `docs/knowledge/lessons/MageLift reference and clean-room policy.md` | IMPORT-06 provenance | ✓ VERIFIED | Phase 4 importers + PatchApplier named |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `init --from-acc/--from-upsun` | `paasimport.MapACC/MapUpsun` | `root.go` RunE | ✓ WIRED | Import + write path + unmapped sidecar |
| Generated YAML | `config.Load` / `config validate` | CLI tests | ✓ WIRED | Happy-path validate executed |
| Foreign `--config` | Loud reject | `config.Load` schemaVersion probe | ✓ WIRED | Shared by validate/deploy load path |
| `build.staticContent.strategy/threads` | PHP SCD argv | plan → protocol → LifecyclePlan | ✓ WIRED | Go + PHP tests assert `-s`/`-j` |
| `composer install` | `PatchApplier` | NativePreparation discover/apply | ✓ WIRED | Alpha-ordered `m2-hotfixes/*.patch` |
| ece-parity closed rows | Shipped code | docs ↔ PatchApplier/SCD/allowlist | ✓ WIRED | Closed claims match 04-03/04/05 surfaces |
| `check-clean-room.sh` | IMPORT-06 | Makefile `verify` | ✓ WIRED | Script exit 0 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Importer YAML | fixture `.magento*` / `.platform*` | Disk fixtures → mapper | Yes — non-empty mapped fields (name, php, domain, SCD, cron) | ✓ FLOWING |
| Unmapped sidecar | residual keys | `Result.Unmapped` → `RenderUnmappedReport` | Yes — named keys in report | ✓ FLOWING |
| SCD argv | strategy/threads | magelift.yaml → plan → protocol | Yes — compact/standard + thread counts in tests | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Focused Go packages | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ ./internal/config/ ./internal/build/plan/ ./internal/build/runner/ -count=1` | 226 passed / 5 packages | ✓ PASS |
| CLI import contract | `go test ./internal/cli/ -run 'FromAcc\|FromUpsun\|ConfigOut\|Unmapped\|Yes\|Mutual\|Foreign\|…'` | All listed cases PASS | ✓ PASS |
| ece-parity blank-row assert | 04-06 `python3 -c '… assert not bad …'` | `rows 35 bad 0` | ✓ PASS |
| Clean-room | `bash scripts/check-clean-room.sh` | `ok (no vendored…)` exit 0 | ✓ PASS |
| PHP build gates | `make php-test` | 110 tests OK (+ analyse/psalm green) | ✓ PASS |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| N/A | — | Phase declares script asserts, not `scripts/*/tests/probe-*.sh` | SKIPPED |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| IMPORT-01 | 01,02,03 | `init --from-acc` → valid YAML | ✓ SATISFIED | CLI + mapper tests |
| IMPORT-02 | 02,03 | `init --from-upsun` → valid YAML | ✓ SATISFIED | Upsun fixture + CLI |
| IMPORT-03 | 03 | Map app/services/routes/cron; report unmapped | ✓ SATISFIED | Unmapped sidecar + domain tests |
| IMPORT-04 | 01,03 | Schema/`config validate` without hand edits | ✓ SATISFIED | Load + validate on generated YAML (schema derived from same model as Load) |
| IMPORT-05 | 01 | Reviewable file; foreign schema never deploy input | ✓ SATISFIED | Foreign Load/validate reject |
| IMPORT-06 | 01,05,06 | Clean-room policy | ✓ SATISFIED | knowledge + provenance + check-clean-room |
| ECE-01 | 06 | Parity matrix closed or intentional-gap | ✓ SATISFIED | ece-parity.md assert |
| ECE-02 | 05 | m2-hotfixes-style patches | ✓ SATISFIED | PatchApplier + php-test |
| ECE-03 | 04 | SCD locales/themes/strategy/threads | ✓ SATISFIED | Go plan + PHP LifecyclePlan tests |
| ECE-04 | 03,06 | Env allowlist mapped or intentional-gap | ✓ SATISFIED | D-07 docs + allowlist tests |

No orphaned Phase 4 requirements — all IMPORT-01..06 and ECE-01..04 appear in plan `requirements:` fields.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | No `TBD`/`FIXME`/`XXX` in phase-touched paasimport/PatchApplier/ece-parity/check-clean-room | — | None |
| `internal/paasimport/map.go` | ~67 | `defaultEncryptionKeyPlaceholder` ARN | ℹ️ Info | Intentional secret-ref shape; plaintext crypt never written (asserted) |

**Notes (non-blocking):**
- No dedicated `magelift deploy --config .magento.app.yaml` CLI test; rejection is via shared `options.load()` → `config.Load` (covered by foreign Load/validate tests).
- Unmapped fixture asserts hooks/cron/env residuals; exotic unmapped `services.*` is coded (`mapServices`) but not fixture-asserted — supported path proves service/relationship mapping.

### Human Verification Required

None for Phase 4 goal closure.

- Execute-time `checkpoint:decision` items (D-03/D-04/D-05) were locked during plan execution (04-02/04-03 SUMMARYs) and are covered by automated CLI tests.
- Optional live soak (`MAGELIFT_IMPORT_FIXTURE_*`) is D-02 maintainer-optional; soak tests skip cleanly when unset (`TestSoakACCSkipsWhenUnset` / Upsun).

### Gaps Summary

No gaps. Phase goal achieved in codebase with behavioral evidence.

### Remaining HUMAN_GATEs

**None for Phase 4.**

(Deferred elsewhere, not Phase 4 blockers: Phase 1 hosted Actions QUALITY-06; Phase 5 dump/media; optional ACC/Upsun soak exports.)

---

_Verified: 2026-07-29T15:49:28Z_  
_Verifier: Claude (gsd-verifier)_
