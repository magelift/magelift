# Phase 7 Plan Check — GCP Certification

**Checked:** 2026-07-30 (re-check after SC1/SC2 harden on 07-06 + 07-03)  
**Plans verified:** 7 (`07-01` … `07-07`)  
**Verdict:** **PLAN CHECK PASSED**

---

## Mandatory gates (orchestrator)

| Gate | Result |
|------|--------|
| `07-VALIDATION.md` present | ✅ |
| RESEARCH Open Questions resolved | ✅ (Q1–Q6 + E1–E3 all inline `RESOLVED`; E1–E3 → execute prereqs) |
| No literal `&amp;&amp;` / `&amp;` in plans | ✅ (literal `&&` only in `<automated>`; zero HTML entities) |
| CONTEXT D-01..D-06 covered | ✅ (see decision table) |
| Execute prereqs ADC + Cloudflare (E1–E2) | ✅ (`07-06` checkpoint:human-action + `user_setup` + destroy-when-done E3) |
| Serial `GOMAXPROCS=1 GOFLAGS=-p=1` on all `go test` verifies | ✅ |
| GCP-01..06 (+ MIGRATE-04) in plan `requirements` | ✅ |
| Prior SC1/SC2 blockers closed | ✅ (see Goal-backward) |

---

## Coverage Summary

| Requirement | Plans | Status |
|-------------|-------|--------|
| GCP-01 | 01, 06 | Covered — live WIF token exchange hard (Act or gcloud STS) |
| GCP-02 | 01, 06 | Covered — live Composer SM write+read mandatory (no catalog softener) |
| GCP-03 | 03, 06 | Covered |
| GCP-04 | 03, 06 | Covered |
| GCP-05 | 02, 03, 06 | Covered |
| GCP-06 | 07 | Covered (evidence-gated) |
| MIGRATE-04 | 04, 05, 06, 07 | Covered (DNS + managed dump) |

| Decision | Plans | Status |
|----------|-------|--------|
| D-01 project `digital-lab-341608` / `europe-west1` / `mlgcpwt` | 01 (parameterized), 06 | Covered |
| D-02 single create-once + force_clean + matrix | 03, 06 | Covered |
| D-03 managed dump cell + journal imported | 03, 04, 06 | Covered |
| D-04 Cloudflare DNS host + Zone.DNS token | 03, 05, 06 | Covered |
| D-05 WIF + Act-only / no SA keys | 01, 03 (`bootstrap:wif` + exchange), 06 | Covered |
| D-06 certify only with SC1–SC5 evidence | 01/02 honesty, 07 gate | Covered |

Deferred Ideas excluded: Phase 8 attach, multi-region GCP, hosted GH Actions required check (Act-only retained).

---

## Plan Summary

| Plan | Tasks | Files | Wave | depends_on | Status |
|------|-------|-------|------|------------|--------|
| 01 | 2 | 11 | 1 | [] | Valid (scope warning) |
| 02 | 2 | 3 | 2 | 07-01 | Valid |
| 03 | 2 | 6 | 3 | 07-01, 07-02, 07-04, 07-05 | Valid (catalog verify warning) |
| 04 | 2 | 5 | 1 | [] | Valid |
| 05 | 2 | 4 | 1 | [] | Valid |
| 06 | 3 (2 checkpoint + 1 auto) | 3 | 4 | 07-01..07-05 | Valid — SC1/SC2 hardened |
| 07 | 2 | 6 | 5 | 07-06 | Valid |

Dependency graph: acyclic; waves consistent (`02←01`, `03←01/02/04/05`, `06←01..05`, `07←06`). Wave-1 file sets (01 / 04 / 05) do not overlap. `day2.go` shared by 01→02 via `depends_on`.

`verify.plan-structure`: valid on all 7 plans (0 errors).

---

## Dimension Results

| # | Dimension | Result |
|---|-----------|--------|
| 1 | Requirement coverage | ✅ PASS |
| 2 | Task completeness | ✅ PASS |
| 3 | Dependency correctness | ✅ PASS |
| 4 | Key links planned | ✅ PASS (cells → dump/DNS/cost/matrix → docs; SC1/SC2 hard in 06) |
| 5 | Scope sanity | ⚠️ WARNING (01: 11 files) |
| 6 | Verification derivation | ✅ PASS (user-observable truths; 07-07 evidence gate) |
| 7 | Context compliance | ✅ PASS (D-01..D-06 honored; deferred excluded) |
| 7b | Scope reduction | ✅ PASS (prior soft language removed; see re-check) |
| 7c | Architectural tier | ✅ PASS (WIF/SM/cost/dump → gcp/dumpimport; DNS → ops script; certify → docs) |
| 8 | Nyquist compliance | ✅ PASS (see table) |
| 9 | Cross-plan data contracts | ✅ PASS |
| 10 | `.cursor/rules/` compliance | ✅ PASS (`serial-builds-only.mdc` / AGENTS.md; no parallel builds in live pass) |
| 11 | Research resolution | ✅ PASS (all Q/E resolved; heading lacks `(RESOLVED)` — info) |
| 12 | Pattern compliance | ⏭ SKIPPED (no `07-PATTERNS.md`) |

### Dimension 8: Nyquist Compliance

| Task | Plan | Wave | Automated Command | Status |
|------|------|------|-------------------|--------|
| 07-01-01 | 01 | 1 | serial `go test` bootstrap/secrets/cli + ops; deferred-wif assert | ✅ |
| 07-01-02 | 01 | 1 | `rg` Act workflow + no private key | ✅ |
| 07-02-01 | 02 | 2 | serial `go test` cost/ops + cli Cost | ✅ |
| 07-02-02 | 02 | 2 | full cost/ops + `! rg unsupportedCost` | ✅ |
| 07-04-01 | 04 | 1 | serial `go test` dumpimport `-run Kube\|Runner\|…` | ✅ |
| 07-04-02 | 04 | 1 | full `./internal/dumpimport/` | ✅ |
| 07-05-01 | 05 | 1 | cutover `--dry-run` + docs `rg` | ✅ |
| 07-05-02 | 05 | 1 | mocked curl shell test | ✅ |
| 07-03-01 | 03 | 3 | dry-run harness shape + cell catalog `rg` | ✅ |
| 07-03-02 | 03 | 3 | dry-run + checkpoint/evidence `rg` | ✅ |
| 07-06-01 | 06 | 4 | checkpoint:human-action (ADC/CF/destroy) | ✅ N/A |
| 07-06-02 | 06 | 4 | checkpoint:decision (option-a) | ✅ N/A |
| 07-06-03 | 06 | 4 | matrix PASS + WIF/STS/Act + Composer/SM `rg` + scratch force_clean | ✅ |
| 07-07-01 | 07 | 5 | matrix PASS + certified docs `rg` | ✅ |
| 07-07-02 | 07 | 5 | REQUIREMENTS/STATE `rg` | ✅ |

Sampling: no 3 consecutive implementation tasks without `<automated>`.  
Wave 0: no `MISSING` stubs — tests created in-plan.  
`07-VALIDATION.md` exists with Validation Architecture upstream in RESEARCH.  
Overall Nyquist: ✅ PASS

---

## Prior blockers — re-check

| Prior blocker | Status | Evidence |
|---------------|--------|----------|
| SC1 WIF token exchange hard (not Ensure-only) | **CLOSED** | `07-03` cell `bootstrap:wif` requires Act or gcloud STS/WIF exchange — not Ensure-only. `07-06` must_haves + task action + done + verify `rg` for WIF/token exchange/STS/Act. |
| SC2 Composer SM write/read hard (no “if in catalog”) | **CLOSED** | `07-06` must_haves: “mandatory (not optionalized by catalog)”; action forbids softening; done + verify `rg` Composer/Secret Manager/gcp-secret-manager/SM write\|read. |

No remaining scope-reduction language that weakens SC1/SC2.

---

## Warnings (non-blocking)

```yaml
issues:
  - plan: "07-01"
    dimension: scope_sanity
    severity: warning
    description: >
      Plan 01 lists 11 files_modified (warning at 10; blocker at 15). WIF + SM
      Get + Act workflow + docs in one wave-1 plan.
    fix_hint: >
      Optional split: identity/WIF vs Composer SM+Act docs — not required if
      executor stays serial and commits per task.

  - plan: "07-03"
    dimension: key_links_planned
    severity: warning
    description: >
      Task 1 action requires bootstrap:wif with token-exchange proof, but
      <automated> rg only matches day2|deploy|migrate|cost|cutover — not
      bootstrap:wif. Catalog could omit SC1 cell and still pass dry-run verify.
      SC2 Composer SM write+read is hard in 07-06 via build/deploy paths but
      07-03 must_haves/catalog do not annotate day2:secrets + deploy:candidate
      as the SC2 evidence producers (RESEARCH Pattern 2).
    fix_hint: >
      Add bootstrap:wif to catalog rg; optionally note in cells/docs that
      day2:secrets (SM write) + deploy Composer gcp-secret-manager:// read
      produce SC2 matrix rows. Fail-closed remains on 07-06 done/verify.

  - plan: null
    dimension: research_resolution
    severity: info
    description: >
      07-RESEARCH.md heading is `## Open Questions` without `(RESOLVED)` suffix;
      body states ALL RESOLVED and every Q/E row is marked RESOLVED.
    fix_hint: "Rename heading to `## Open Questions (RESOLVED)` on next research touch."

  - plan: "07-06"
    dimension: nyquist_compliance
    severity: warning
    description: >
      Live create-once + PSA soak feedback latency (~25–40m) exceeds 30s guideline;
      expected for paid pass and already constrained to wave 4 after offline plans.
    fix_hint: "Keep as-is; do not move live work earlier. Abort under memory pressure per AGENTS.md."

  - plan: "07-01"
    dimension: verification_derivation
    severity: info
    description: >
      Task 1 automated uses `(grep -c … || true) | awk` to assert zero deferred
      WIF. Not the swallowed-default anti-pattern (awk fails closed on count>0),
      but dense — easy to mis-edit into a always-pass check.
    fix_hint: >
      Prefer `! rg -n 'wif.*deferred|"wif": "deferred"' …` (or dedicated unit
      assert) without `|| true`.
```

---

## Goal-backward (phase success criteria)

| SC | Plan evidence | Status |
|----|---------------|--------|
| SC1 WIF + CI auth no SA keys | 01 Act/docs + 03 bootstrap:wif exchange + 06 hard Act/STS evidence | ✅ Planned |
| SC2 Composer SM write/read loud | 01 Get + existing Set; 06 mandatory write+read (no catalog softener) | ✅ Planned |
| SC3 day2 logs/exec/secrets/state/health | 03 cells + 06 live | ✅ Planned |
| SC4 deploy migrate→cutover→health→record | 03 deploy cell + 06 | ✅ Planned |
| SC5 cost + certified docs | 02 estimator + 06 cost cell + 07 gate | ✅ Planned |
| MIGRATE-04 DNS + managed dump | 04/05 offline + 06 cells + 07 close | ✅ Planned |
| Teardown force_clean + PSA + DNS cleanup | 06 EXIT + checkpoints | ✅ Planned |
| E1–E3 ADC / CF / destroy-when-done | 06 human-action + user_setup | ✅ Planned |

---

## Structured Issues

```yaml
issues:
  - plan: "07-01"
    dimension: scope_sanity
    severity: warning
    description: "11 files_modified exceeds warning threshold (10)"
    fix_hint: "Optional split; not required before execute"

  - plan: "07-03"
    dimension: key_links_planned
    severity: warning
    description: "Catalog verify omits bootstrap:wif; SC2 cell annotation implicit"
    fix_hint: "Tighten 07-03 rg + SC2 cell notes; 07-06 remains fail-closed"

  - plan: "07-06"
    dimension: nyquist_compliance
    severity: warning
    description: "Live pass latency ~25–40m"
    fix_hint: "Keep wave-4 only"

  - plan: null
    dimension: research_resolution
    severity: info
    description: "Open Questions heading missing (RESOLVED) suffix"
    fix_hint: "Cosmetic rename on next research edit"

  - plan: "07-01"
    dimension: verification_derivation
    severity: info
    description: "Dense deferred-WIF count pipeline"
    fix_hint: "Prefer ! rg fail-closed form"
```

---

## Recommendation

**0 blocker(s).** Prior SC1/SC2 scope-reduction blockers are closed.

Plans verified. Run `/gsd-execute-phase 7` to proceed.

Optional polish (non-blocking): add `bootstrap:wif` to 07-03 catalog `rg`; annotate SC2 producers on day2:secrets/deploy cells.
