# Phase 8 Plan Check — Brownfield Attach & Tag Day

**Checked:** 2026-07-30  
**Plans verified:** 6 (`08-01` … `08-06`)  
**Verdict:** **PLAN CHECK PASSED**

---

## Mandatory gates (orchestrator)

| Gate | Result |
|------|--------|
| `08-VALIDATION.md` present | ✅ |
| RESEARCH Open Questions resolved | ✅ (5/5 inline `RESOLVED`; Cloud SQL / GCP / ADC gated) |
| No literal `&amp;&amp;` / `&amp;` in plans | ✅ (literal `&&` only in `<automated>`) |
| CONTEXT D-01..D-05 covered | ✅ (see decision table) |
| Deferred Ideas excluded | ✅ (no fake GCP certify; no multi-cloud attach impl) |
| Serial `GOMAXPROCS=1 GOFLAGS=-p=1` on all `go test` verifies | ✅ |
| ATTACH-01..04, RELEASE-05 in plan `requirements` | ✅ |

---

## Coverage Summary

| Requirement | Plans | Status |
|-------------|-------|--------|
| ATTACH-01 | 01, 06 | Covered — VPC ADOPT preview + zero-create + refuse (network) |
| ATTACH-02 | 02, 03, 06 | Covered — config/Spec + database.Existing + stack wire; Cloud SQL deferred on board |
| ATTACH-03 | 01, 04, 06 | Covered — unified ADOPT report + refuse network+DB (test-asserted) |
| ATTACH-04 | 04 (offline half), 05, 06 | Covered — docs limits/detach + offline no-Delete + optional live confirm |
| RELEASE-05 | 06 | Covered — board Closed/Deferred/Pending→07; REQUIREMENTS settled |

| Decision | Plans | Status |
|----------|-------|--------|
| D-01 preview ADOPT / apply without replace | 01, 02, 03, 04 | Covered |
| D-02 refuse-before-mutate naming resource | 01, 04 | Covered |
| D-03 attach limits + detach intact | 05 (+ 04 offline proof, 06 live) | Covered |
| D-04 Floci/mocks first; free-tier AWS when ADC | 06 | Covered |
| D-05 tag board honesty; Act-only CI; no fake GCP | 06 | Covered |

Deferred Ideas excluded: fake-certifying GCP before 07 live; multi-cloud attach beyond AWS free-tier confirmation. Discretion honored: YAML field names (`existing.database` + secretArn/endpoint); detach = documented manual un-adopt (no CLI).

---

## Plan Summary

| Plan | Tasks | Files | Wave | depends_on | Status |
|------|-------|-------|------|------------|--------|
| 01 | 2 (1 tracer + 1 auto) | 5 | 1 | [] | Valid |
| 02 | 2 | 8 | 1 | [] | Valid |
| 03 | 2 | 4 | 2 | 08-02 | Valid |
| 04 | 2 | 4 | 3 | 08-01, 08-03 | Valid |
| 05 | 2 | 8 | 4 | 08-03, 08-04 | Valid |
| 06 | 3 (2 auto + 1 checkpoint) | 4 | 5 | 08-01..08-05 | Valid |

Dependency graph: acyclic; wave = max(dep waves)+1. Wave-1 file sets (`adopt`/`lifecycle`/`network_test` vs config/schema/Spec) do not overlap. `adopt.go` / `lifecycle.go` shared 01→04 via `depends_on`.

`verify.plan-structure`: valid on all 6 plans (0 errors).

---

## Dimension Results

| # | Dimension | Result |
|---|-----------|--------|
| 1 | Requirement coverage | ✅ PASS |
| 2 | Task completeness | ✅ PASS |
| 3 | Dependency correctness | ✅ PASS |
| 4 | Key links planned | ✅ PASS (Spec.Existing → AdoptReport → CLI; Refuse → Update/Destroy; config → PlanFromConfig → database.Existing → IAM) |
| 5 | Scope sanity | ✅ PASS (2–3 tasks/plan; max 8 files) |
| 6 | Verification derivation | ✅ PASS (user-observable ADOPT/refuse/docs/board truths) |
| 7 | Context compliance | ✅ PASS (D-01..D-05; deferred excluded) |
| 7b | Scope reduction | ✅ PASS (Cloud SQL / multi-cloud = CONTEXT deferred + REQUIREMENTS deferral, not silent shrink of locked D-XX) |
| 7c | Architectural tier | ✅ PASS (config/PlanFromConfig + components + CLI refuse; docs/board; paid AWS = HUMAN_GATE) |
| 8 | Nyquist compliance | ✅ PASS (see table; latency warning) |
| 9 | Cross-plan data contracts | ✅ PASS (`Existing.Database` + secret/endpoint 02→03→04; reference-without-own preserved) |
| 10 | `.cursor/rules/` compliance | ✅ PASS (`serial-builds-only.mdc` / AGENTS.md on all verifies) |
| 11 | Research resolution | ✅ PASS |
| 12 | Pattern compliance | ⏭ SKIPPED (no `08-PATTERNS.md`) |

### Dimension 8: Nyquist Compliance

| Task | Plan | Wave | Automated Command | Status |
|------|------|------|-------------------|--------|
| 08-01-01 | 01 | 1 | `go test …stack -run Adopt\|Refuse\|ExistingNetwork` + cli Preview/Adopt + `rg` AdoptReport | ✅ |
| 08-01-02 | 01 | 1 | `go test …network -run ExistingNetworkUsesImported` + stack Adopt\|Refuse | ✅ |
| 08-02-01 | 02 | 1 | `go generate ./internal/config` + config tests + `rg` existing.database | ✅ |
| 08-02-02 | 02 | 1 | `go test …stack -run ExistingDatabase\|PlanFromConfig…` + config | ✅ |
| 08-03-01 | 03 | 2 | `go test …database -run Existing\|…` + `rg` Args.Existing | ✅ |
| 08-03-02 | 03 | 2 | database Existing + stack ExistingDatabase\|ComposesPreview | ✅ |
| 08-04-01 | 04 | 3 | stack Adopt\|Refuse\|Adopted + cli Adopt\|Preview | ✅ |
| 08-04-02 | 04 | 3 | stack Adopt\|Refuse\|Detach + database/network Existing | ✅ |
| 08-05-01 | 05 | 4 | `test -f docs/brownfield-attach.md` + `rg` detach/ADR/cross-links | ✅ |
| 08-05-02 | 05 | 4 | stack Adopt\|Refuse\|Detach + doc cites offline proof | ✅ |
| 08-06-01 | 06 | 5 | quick map Existing\|Adopt\|Refuse\|Detach + evidence file | ✅ |
| 08-06-02 | 06 | 5 | checkpoint:human-verify (AWS ADC / free-tier confirm) | ✅ N/A |
| 08-06-03 | 06 | 5 | release-readiness Closed/Deferred/Pending gates + REQUIREMENTS `rg` | ✅ |

Sampling: Wave 1–5 — no 3 consecutive implementation tasks without `<automated>`.  
Wave 0: no `MISSING` stubs — adopt/DB Existing tests created in-plan (tdd/tracer).  
Overall: ✅ PASS

---

## Warnings (non-blocking)

```yaml
issues:
  - plan: "08-06"
    dimension: nyquist_compliance
    severity: warning
    description: >
      Quick / multi-package verify map is estimated 60–180s (VALIDATION.md),
      above the 30s feedback-latency preference. Serial GOMAXPROCS=1 is required
      by AGENTS.md — acceptable tradeoff.
    fix_hint: >
      Keep package-scoped -run filters per task during execution; reserve the
      four-package map for wave/plan boundaries and 08-06-01.

  - plan: "08-06"
    task: 3
    dimension: verification_derivation
    severity: warning
    description: >
      RELEASE-05 REQUIREMENTS settlement verify ends with
      `rg … ATTACH-0|RELEASE-05|Cloud SQL | head -20` — dumps lines but does not
      fail if ATTACH/RELEASE rows remain Pending or lack Complete/deferral text.
      Board Closed/Deferred/Pending checks are solid; REQUIREMENTS half is soft.
    fix_hint: >
      Add assertions that ATTACH-01..04 and RELEASE-05 match Complete|Deferred
      (and Cloud SQL deferral appears), e.g. rg -c gates with awk fail-closed.

  - plan: null
    dimension: research_resolution
    severity: info
    description: >
      08-RESEARCH.md heading is `## Open Questions` without `(RESOLVED)` suffix;
      all five bullets are inline RESOLVED. Cosmetic only.
    fix_hint: "Rename heading to `## Open Questions (RESOLVED)` on next research touch."

  - plan: "08-04"
    dimension: requirement_coverage
    severity: info
    description: >
      Plan 04 must_haves/actions deliver ATTACH-04 offline detach proof but
      frontmatter requirements list only ATTACH-03. Coverage still lands via
      08-05/08-06.
    fix_hint: "Optional: add ATTACH-04 to 08-04 requirements for traceability."
```

---

## Goal-backward (phase success criteria)

| SC | Plan evidence | Status |
|----|---------------|--------|
| SC1 VPC preview ADOPT + apply without replace | 01 (AdoptReport + refuse + ExistingNetwork zero-create) | Planned |
| SC2 adopt managed DB; preview no destructive change | 02+03 Existing path; 04 ADOPT+refuse | Planned (AWS RDS; Cloud SQL deferred) |
| SC3 refuse mutate/destroy adopted — test-asserted | 01 network; 04 network+DB | Planned |
| SC4 documented limits + detach leaves resource intact | 05 docs; 04 offline no-Delete; 06 live or ADC gate | Planned |
| SC5 every gate/requirement Closed or Deferred | 06 board + REQUIREMENTS; GCP Pending→07; Act-only Deferred | Planned |

---

## Recommendation

**0 blockers.** Plans will achieve Phase 8 goal under D-01..D-05 and ATTACH-01..04 + RELEASE-05, with honest Phase 7 GCP Pending and optional AWS ADC HUMAN_GATE.

Proceed: `/gsd-execute-phase 8`
