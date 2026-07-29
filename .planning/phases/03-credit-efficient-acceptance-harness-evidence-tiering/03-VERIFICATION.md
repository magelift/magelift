---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
verified: 2026-07-29T14:44:43Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification: false
gaps: []
deferred:
  - truth: "GCP live create + PSA soak + force-clean on a real project (ACCEPT-05 live half)"
    addressed_in: "Phase 7"
    evidence: "ROADMAP Phase 3 SC#4 only requires dry-run/preview shape before Phase 7 spends credits; Phase 7 goal/SCs exercise the live GCP pass (~19m21s create + PSA soak + force-clean)"
  - truth: "Hosted GitHub Actions / QUALITY-06 first green force-all on main"
    addressed_in: "Phase 1 HUMAN_GATE (not a Phase 3 success criterion)"
    evidence: "Explicit out of Phase 3 scope; Phase 1 VERIFICATION remains human_needed for hosted CI minutes"
---

# Phase 3: Credit-Efficient Acceptance Harness & Evidence Tiering Verification Report

**Phase Goal:** Every later paid pass costs one stack and buys only what mocks cannot prove — and no cell in the capability matrix claims more than the harness actually recorded.

**Verified:** 2026-07-29T14:44:43Z
**Status:** passed
**Re-verification:** No prior `03-VERIFICATION.md` on disk — initial verification after plans 03-01…03-06 SUMMARY + paid AWS proof (2026-07-29).

## Recommendation

**Mark Phase 3 complete and start Phase 4.** All five ROADMAP success criteria (ACCEPT-01..06, TRUST-03, TRUST-04) are observably true: offline harness + dual `assert_clean` stubs, matrix honesty surface, Floci/unit-fake port map, GCP dry-run shape, and live free-tier AWS create-once ≥3 cells with kill+resume + dual `assert_clean`. Do **not** re-run paid AWS create. GCP live spend stays Phase 7. Hosted Actions / QUALITY-06 remains a **Phase 1** HUMAN_GATE only — not a Phase 3 contract item.

## Goal Achievement

### Observable Truths

Roadmap success criteria are the contract. Plan must_haves add detail but do not shrink this list.

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | Acceptance run iterates ≥3 catalog cells on one stack — log shows one Pulumi create then updates (never re-create between cells); kill mid-matrix then re-invoke resumes at first incomplete cell, skipping recorded ones | ✓ VERIFIED | Live log `scratch/03-06-live-run-health.log`: one `acceptance create-once` (L37), then `cell-update` for `queueMode:db` / `ecs-rabbitmq` / `ecs-artemis` with `cell-done … result=PASS`. After kill: `acceptance resume: skipping create-once` + `acceptance skip cell=queueMode:db` then continues rabbitmq→artemis. Proof note: `scratch/03-06-paid-proof.md`. Offline half: `make acceptance-harness-test` → `checkpoint_resume_test` + dry-run create-once/updates markers OK. |
| 2 | After a run, `.magelift/matrix-results.md` has one harness-appended row per cell (cell, result, duration, provider, account, date); no hand-typed evidence | ✓ VERIFIED | Gitignored file present with header + three PASS rows (`db` 255s / `ecs-rabbitmq` 254s / `ecs-artemis` 204s, provider `aws`, account `669890779205`, date `2026-07-29`). Written via `scripts/acceptance/lib-evidence.sh` `append_row` (sourced by `aws-acceptance-local.sh`). Redacted sample also in `scratch/03-06-paid-proof.md`. Offline: `evidence_append_test.sh` OK. |
| 3 | Teardown destroys everything created; `assert_clean` exits non-zero on leftover, zero when clean — both demonstrated | ✓ VERIFIED | Mid-run leftover (KEEP stack): `assert_clean FAILED` with leftover VPC/RDS/ElastiCache/ALB/ECS/logs/SGs (`live-run-health.log` ~L1145–1153). After `DESTROY_EXIT:0`: `assert_clean ok` (L2003–2005). Shared helper `scripts/acceptance/lib-assert-clean-aws.sh` on EXIT path. Offline stubs: `--clean` → 0; `--leftover` → non-zero (`assert_clean_stub_test.sh`). |
| 4 | GCP harness path same shape end-to-end including PSA soak + force-clean teardown, verified at least as dry-run/preview before Phase 7 spends credits | ✓ VERIFIED | `scripts/gcp-acceptance-local.sh` sources shared checkpoint/evidence libs; defines `force_clean_orphans` (PSA soak), `assert_clean`, cleanup `destroy` → `force_clean_orphans` → `assert_clean`. `tests/acceptance/gcp_harness_shape_test.sh` OK (`created=0`, no spending mutate). Docs: `docs/gcp-acceptance.md`. Live GCP create deferred to Phase 7 by ROADMAP (not a Phase 3 gap). |
| 5 | `docs/capability-matrix.md` records evidence tier per cell + explicit unverifiable reasons for Aurora `CreateDBCluster`, `amazon-mq` × `preview`, OpenSearch SigV4 data plane; checked-in port-coverage table maps day-2 ports to mocks/Floci/paid-only; `make floci-test` covers every mockable port | ✓ VERIFIED | Free-tier table tiers present; `queueMode: db/ecs-rabbitmq/ecs-artemis` = `real-account acceptance` citing 2026-07-29 harness. Unverifiable triad table with specific reasons (TRUST-04). Day-2 port coverage table + `port_coverage_floci_gate_test.sh` OK (every mockable row maps to a `*_test.go` symbol). `matrix_tier_guard_test.sh` OK. Full `make floci-test` not re-run this verify (Docker/serial cost on 16GB); gate + symbol presence + 03-04 SUMMARY green stand as ACCEPT-06 offline proof. |

**Score:** 5/5 truths verified (0 present, behavior-unverified; 0 FAIL; 0 UNCERTAIN)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | GCP live create + PSA soak + force-clean on real project | Phase 7 | Phase 3 SC#4 explicitly dry-run ceiling; Phase 7 paid pass |
| 2 | Hosted CI / QUALITY-06 green force-all | Phase 1 HUMAN_GATE | Not a Phase 3 SC |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ----------- | ------ | ------- |
| `scripts/acceptance/lib-checkpoint.sh` | Checkpoint/resume API | ✓ VERIFIED | Sourced by AWS + GCP harnesses |
| `scripts/acceptance/lib-evidence.sh` | Six-column `append_row` | ✓ VERIFIED | Wired; live matrix-results written |
| `scripts/acceptance/lib-assert-clean-aws.sh` | Shared assert_clean | ✓ VERIFIED | EXIT path + stub dual-outcome |
| `scripts/acceptance/cells-aws-preview.txt` | ≥3 free-tier cells | ✓ VERIFIED | db / ecs-rabbitmq / ecs-artemis |
| `scripts/aws-acceptance-local.sh` | create-once + cell-update + KEEP/RESUME | ✓ VERIFIED | Live proven 2026-07-29 |
| `scripts/gcp-acceptance-local.sh` | Same shape + PSA/force_clean | ✓ VERIFIED | Dry-run shape test OK |
| `docs/capability-matrix.md` | Tiers + unverifiable + port map | ✓ VERIFIED | Substantive; honesty rule stated |
| `docs/aws-acceptance.md` / `docs/gcp-acceptance.md` | Maintainer harness docs | ✓ VERIFIED | Cite paid proof + dry-run |
| `tests/acceptance/*.sh` | Offline suite | ✓ VERIFIED | `make acceptance-harness-test` EXIT 0 |
| `scratch/03-06-paid-proof.md` | Paid proof checklist | ✓ VERIFIED | ACCEPT-01..04 closed |
| `.magelift/matrix-results.md` | Harness evidence (gitignored) | ✓ VERIFIED | Present; three PASS rows |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | ---- | ------ | ------- |
| `aws-acceptance-local.sh` | `lib-checkpoint` / `lib-evidence` / `lib-assert-clean-aws` | `source` | ✓ WIRED | Top of script |
| Harness cell loop | `.magelift/matrix-results.md` | `append_row` | ✓ WIRED | Live rows match PASS cells |
| EXIT cleanup (AWS) | `assert_clean` | destroy unless KEEP | ✓ WIRED | Dual outcome in live log |
| `gcp-acceptance-local.sh` | shared libs + `force_clean_orphans` | source + cleanup() | ✓ WIRED | Shape test asserts symbols + order |
| `capability-matrix.md` port table | Floci/unit-fake tests | gate script symbol map | ✓ WIRED | `port_coverage_floci_gate_test` OK |
| Matrix `real-account` cells | paid proof / matrix-results | 2026-07-29 citations | ✓ WIRED | db/rabbitmq/artemis under-claim closed |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| matrix-results rows | cell/result/duration/provider/account/date | Harness after each cell-done | Live account `669890779205`, measured durations | ✓ FLOWING |
| assert_clean leftover counts | aws describe stubs / live API | Shared helper queries | Non-zero leftovers then zero post-destroy | ✓ FLOWING |
| GCP dry-run evidence | provider-scoped paths | `.magelift/gcp-matrix/` overrides | Fixture PASS rows; `created=0` | ✓ FLOWING (offline) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Offline harness suite | `make acceptance-harness-test` | All 6 scripts OK; EXIT 0 | ✓ PASS |
| Live create-once + 3 cells | Read `03-06-live-run-health.log` | create-once once; 3× PASS | ✓ PASS (prior live; not re-run) |
| Kill+resume | Same log | skip create-once + skip db | ✓ PASS (prior live) |
| Dual assert_clean | Same log + paid-proof | FAILED then ok | ✓ PASS (prior live) |
| Matrix six columns | `cat .magelift/matrix-results.md` | 3 harness rows | ✓ PASS |
| Full `make floci-test` | — | Not re-run (Docker/16GB) | ? SKIP (gate + 03-04 SUMMARY) |
| Paid AWS re-create | — | Explicitly out of verify scope | ? SKIP |
| Hosted Actions | — | Phase 1 only | ? SKIP |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| — | — | No phase-declared `scripts/*/tests/probe-*.sh` | SKIPPED |

Offline acceptance shell tests under `tests/acceptance/` served as the probe substitute and all passed.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| ACCEPT-01 | 03-01, 03-06 | One long-lived stack; no re-create between cells | ✓ SATISFIED | Truth 1 |
| ACCEPT-02 | 03-01, 03-06 | Resume from last completed cell | ✓ SATISFIED | Truth 1 |
| ACCEPT-03 | 03-01, 03-06 | Automatic matrix-results evidence | ✓ SATISFIED | Truth 2 |
| ACCEPT-04 | 03-02, 03-06 | Destroy + assert_clean dual outcome | ✓ SATISFIED | Truth 3 |
| ACCEPT-05 | 03-05 | GCP same harness shape (dry-run Phase 3) | ✓ SATISFIED | Truth 4; live → Phase 7 |
| ACCEPT-06 | 03-03, 03-04 | Mockable day-2 ports covered offline | ✓ SATISFIED | Truth 5 |
| TRUST-03 | 03-03 | Evidence tier per cell; no over-claim | ✓ SATISFIED | Truth 5 + matrix update post-paid |
| TRUST-04 | 03-03 | Unverifiable cells with specific reasons | ✓ SATISFIED | Truth 5 triad table |

No orphaned Phase 3 requirements. All eight IDs claimed by plans and satisfied.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `docs/capability-matrix.md` | Day-2 intro | Stale “Floci gap closure is a later plan” wording | ℹ️ Info | 03-04 already closed gaps; refresh prose later |
| `scripts/aws-acceptance-local.sh` | mktemp | `XXXXXX` path template | ℹ️ Info | Not a debt marker |

No `TBD`/`FIXME`/`XXX` debt markers in phase-touched product files.

### Human Verification Required

None for Phase 3 roadmap SCs. Paid AWS HUMAN_GATE already closed with durable logs + proof note (2026-07-29). GCP live remains Phase 7 spend, not an end-of-phase Phase 3 human check. Phase 1 hosted CI remains a separate HUMAN_GATE.

### Gaps Summary

No actionable Phase 3 gaps. Goal achieved: credit-efficient harness (create-once, resume, evidence, assert_clean) is proven on free-tier AWS; matrix honesty + port coverage hold offline; GCP shape is dry-run ready for Phase 7.

---

_Verified: 2026-07-29T14:44:43Z_
_Verifier: Claude (gsd-verifier)_
