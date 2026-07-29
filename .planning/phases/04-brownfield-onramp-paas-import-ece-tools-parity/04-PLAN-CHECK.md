# Phase 4 Plan Check

**Phase:** 04-brownfield-onramp-paas-import-ece-tools-parity
**Checked:** 2026-07-29 (re-check after blocker fixes)
**Plans verified:** 6 (`04-01` … `04-06`)
**Status:** PLAN CHECK PASSED

## Verdict

## VERIFICATION PASSED

**Issues:** 0 blocker(s), 2 warning(s), 0 info

All prior blockers are fixed. Plans cover IMPORT-01..06 and ECE-01..04, honor CONTEXT D-01..D-07, keep deferred Phase 5–8 out of scope, and have valid structure/deps. Execution may proceed; address warnings during execute if practical.

### Prior blockers — re-verified fixed

| # | Prior blocker | Evidence | Status |
|---|---------------|----------|--------|
| 1 | `04-VALIDATION.md` missing | File exists; `status: draft`, `nyquist_compliant: false` | Fixed |
| 2 | RESEARCH Open Questions unresolved | `## Open Questions (RESOLVED)` with `--config-out`, `application.cron`, QUALITY_PATCHES gap | Fixed |
| 3 | Literal `&amp;&amp;` in 04-04/05/06 verifies | Plans use real `&&`; no `&amp;&amp;` in any `04-0*-PLAN.md` | Fixed |
| 4 | 04-06 ece-parity verify missing `assert not bad` | Task 1 `<automated>` ends with `assert not bad, bad` | Fixed |

---

## Coverage Summary

| Requirement | Plans | Status |
|-------------|-------|--------|
| IMPORT-01 | 01, 02, 03 | Covered |
| IMPORT-02 | 02, 03 | Covered |
| IMPORT-03 | 03 | Covered |
| IMPORT-04 | 01, 03 | Covered |
| IMPORT-05 | 01 | Covered |
| IMPORT-06 | 01, 05, 06 | Covered |
| ECE-01 | 06 | Covered |
| ECE-02 | 05 | Covered |
| ECE-03 | 04 | Covered |
| ECE-04 | 03, 06 | Covered |

| Decision | Plans / mechanism | Status |
|----------|-------------------|--------|
| D-01 fixtures | 01, 03 | Covered |
| D-02 soak skip | 03 | Covered |
| D-03 `--from-acc/--from-upsun` | 01, 02 + `checkpoint:decision` | Covered |
| D-04 refuse/`--yes` + side-file | 02 + `checkpoint:decision` (`--config-out`) | Covered |
| D-05 unmapped sidecar + non-zero | 03 + `checkpoint:decision` | Covered |
| D-06 parity + patches + SCD | 04, 05, 06 | Covered |
| D-07 allowlist | 03, 06 | Covered |

| ROADMAP success criterion | Status |
|---------------------------|--------|
| SC1 ACC+Upsun → validate/schema | Plans 01–03 |
| SC2 unmapped report key-by-key | Plan 03 |
| SC3 reviewable file + foreign reject + clean-room | Plans 01, 06 |
| SC4 ece-parity no blank rows | Plan 06 (`assert not bad`) |
| SC5 php-test patches + SCD | Plans 04, 05 |

Deferred Phase 5–8 / cloud spend / full ece-tools clone: excluded. No scope reduction or deferred-idea creep.

---

## Plan Summary

| Plan | Tasks | Wave | depends_on | Status |
|------|-------|------|------------|--------|
| 04-01 | 2 | 1 | [] | Valid |
| 04-02 | 3 (2 checkpoint + 1 auto) | 2 | 04-01 | Valid |
| 04-03 | 3 (1 checkpoint + 2 auto) | 3 | 04-02, 04-04 | Valid |
| 04-04 | 2 | 1 | [] | Valid |
| 04-05 | 2 | 2 | 04-04 | Valid |
| 04-06 | 2 | 4 | 04-03, 04-05 | Valid |

Wave graph: acyclic; 01∥04 → 02∥05 → 03 → 06. Schema/model touch order (04 before 03) is correct. `verify.plan-structure` → valid for all six.

---

## Dimension Results

| # | Dimension | Result |
|---|-----------|--------|
| 1 | Requirement coverage | PASS |
| 2 | Task completeness | PASS |
| 3 | Dependency correctness | PASS |
| 4 | Key links planned | PASS |
| 5 | Scope sanity | WARNING — 04-01 ~12 files; 04-03 ~14 file entries |
| 6 | Verification derivation | PASS |
| 7 | Context compliance | PASS |
| 7b | Scope reduction | PASS |
| 7c | Architectural tier | PASS |
| 8 | Nyquist compliance | PASS (VALIDATION present; all auto tasks have `<automated>`; no MISSING) |
| 9 | Cross-plan data contracts | PASS |
| 10 | `.cursor/rules/` | PASS — serial `GOMAXPROCS=1 GOFLAGS=-p=1` |
| 11 | Research resolution | PASS |
| 12 | Pattern compliance | SKIPPED (no PATTERNS.md) |

### Dimension 8: Nyquist Compliance

| Task | Plan | Wave | Automated Command | Status |
|------|------|------|-------------------|--------|
| 01-01 | 01 | 1 | `go test … Acc\|FromAcc\|MapACC` | ✅ |
| 01-02 | 01 | 1 | `go test … Foreign\|…` | ✅ |
| 02-01/02 | 02 | 2 | checkpoint:decision | ✅ N/A |
| 02-03 | 02 | 2 | `go test … Init\|FromAcc\|…` | ✅ |
| 03-01 | 03 | 3 | checkpoint:decision | ✅ N/A |
| 03-02 | 03 | 3 | `go test … Unmapped\|Upsun\|…` | ✅ |
| 03-03 | 03 | 3 | `go test … Cron\|Soak\|…` | ✅ |
| 04-01 | 04 | 1 | `go test … && make php-test` | ✅ (latency warning) |
| 04-02 | 04 | 1 | `go test … Static\|Schema\|Build` | ✅ |
| 05-01 | 05 | 2 | `make php-test` | ✅ (latency warning) |
| 05-02 | 05 | 2 | `test -f docs/ece-parity.md && grep …` | ✅ |
| 06-01 | 06 | 4 | python `assert not bad` | ✅ |
| 06-02 | 06 | 4 | `check-clean-room.sh && grep …` | ✅ |

Sampling: every wave has ≥2/3 automated on consecutive implementation windows → ✅  
Wave 0: no `<automated>MISSING</automated>` refs → ✅  
Overall: ✅ PASS

---

## Warnings (should fix; execution may proceed)

**1. [scope_sanity] Plan 04-01 lists ~12 files_modified; 04-03 lists ~14 entries**
- Acceptable for tracer / shared-mapper slices; optional split only if executor context pressure appears.

**2. [nyquist_compliance 8b] `make php-test` likely >30s feedback latency**
- Plans: 04-04 task 1, 04-05 task 1
- Prefer focused PHPUnit filter in task verify when Makefile supports it; keep full `make php-test` for wave/phase gate.

---

## Structured Issues

```yaml
issues:
  - plan: "04-01"
    dimension: scope_sanity
    severity: warning
    description: "files_modified ~12 exceeds warning threshold of 10"
    metrics:
      tasks: 2
      files: 12
    fix_hint: "Optional: leave as tracer or split foreign-schema reject"

  - plan: "04-03"
    dimension: scope_sanity
    severity: warning
    description: "files_modified ~14 entries near warning threshold"
    metrics:
      tasks: 3
      files: 14
    fix_hint: "Acceptable for shared mapper; monitor executor context"

  - plan: "04-04"
    dimension: nyquist_compliance
    severity: warning
    task: 1
    description: "make php-test as task verify likely exceeds 30s feedback latency"
    fix_hint: "Use focused phpunit filter in task verify; reserve full make php-test for wave gate"

  - plan: "04-05"
    dimension: nyquist_compliance
    severity: warning
    task: 1
    description: "make php-test as sole task verify likely exceeds 30s feedback latency"
    fix_hint: "Use focused phpunit filter in task verify; reserve full make php-test for wave gate"
```

---

## Recommendation

0 blockers. Plans verified — proceed with `/gsd-execute-phase 4`.

**PLAN CHECK PASSED**
