---
phase: 01-publishable-baseline-honest-fallbacks
verified: 2026-07-28T14:25:50Z
status: human_needed
score: 4/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 2/5
  gaps_closed:
    - "No non-test file under internal/cloud/aws/runtime/ exceeds 400 lines; runtime_test.go suite proves the split preserved behaviour (QUALITY-07 / criterion 2)"
    - "No-op AcquireLock implementations warn that no lock was taken rather than returning a release function that implies one was (TRUST-02 / criterion 5 AcquireLock clause)"
    - "gofmt -l clean on previously drifted lifecycle_test.go and stack/component_test.go"
  gaps_remaining:
    - "The go job passes on main — genuinely, not by being skipped — with partitioned lint and every make verify target green in CI (QUALITY-06 / criterion 1 outcome)"
  regressions: []
# Criterion 1 remains unmet as an outcome. It is not closable offline (Actions minutes).
# Routed to human_verification / HUMAN_GATE — do NOT spawn another gap-closure plan for CI.
gaps: []
deferred:
  - truth: "F-01-07-1 search-proxy DependsOn skipped for nginx-fpm non-php-fpm task containers"
    addressed_in: "Post-Phase-1 / later correctness fix (not a ROADMAP Phase 1 success criterion)"
    evidence: "01-07-SUMMARY Finding F-01-07-1; 01-10 left untouched by design"
human_verification:
  - test: "When GitHub Actions minutes return, dispatch force-all CI on main and confirm six lint partitions + go-verify + php/docs/workflow-lint all succeed"
    expected: "Durable green run URL; every make verify target executes; docs/lint-policy.md deferred-ci placeholders replaced with real numbers"
    why_human: "Verifier must not burn Actions minutes; hosted outcome cannot be proven offline. Criterion 1 is the sole remaining Phase 1 contract item."
  - test: "Optionally run serial whole-module vs partitioned golangci issue-set diff (A1) in a plain Terminal outside Cursor"
    expected: "Identical issue sets (or documented intentional differences)"
    why_human: "Full golangci on 16GB Mac under Cursor risks OOM/kernel panic per AGENTS.md"
---

# Phase 1: Publishable Baseline & Honest Fallbacks Verification Report

**Phase Goal:** The repository survives a stranger's first read — CI actually runs and passes the verification suite for the first time, the highest-risk file is no longer a single 991-line construction path, every recently fixed bug has a regression guard, and no target can silently pretend to support something it does not.

**Verified:** 2026-07-28T14:25:50Z
**Status:** human_needed
**Re-verification:** Yes — after gap-closure plan 01-10

## Recommendation

**Phase 1 can advance with HUMAN_GATE only for hosted CI.** Offline blockers from the prior verification (QUALITY-07 runtime split, AcquireLock warn, gofmt drift) are closed and spot-checked. Do **not** plan another gap-closure wave for criterion 1 — it is impossible offline while Actions minutes are exhausted. Do **not** start Phase 2 until a human accepts the deferred CI proof gate (or minutes return and a green force-all run is recorded).

## Goal Achievement

### Observable Truths

Roadmap success criteria are the contract. Plan must_haves add detail but cannot shrink this list.

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | `go` job passes on main (partitioned lint + all `make verify` targets in CI); A1 partitioning parity verified | ? UNCERTAIN (deferred-ci) | Workflow shape still sound (`.github/workflows/ci.yml`: `cache-prime`, lint matrix, `go-verify` in `result.needs`). `.golangci.yml` has no `run.concurrency` / `run.timeout`. Union-coverage guard passes (`go test ./internal/lintcoverage/`). `gofmt -l` clean on previously drifted files. **No hosted green proof** — Actions minutes exhausted; `docs/lint-policy.md` still has `deferred-ci` placeholders; A1 issue-set diff not recorded. Outcome unmet; implementation present — HUMAN_GATE only. |
| 2 | No non-test `internal/cloud/aws/runtime/` file >400 lines; suite proves split preserved behaviour | ✓ VERIFIED | `runtime.go` deleted. Seven non-test files, max `containers.go` 226 lines (all ≤400). `New` in `component.go`. Full `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/runtime/ -count=1` ok (includes SigV4 + 01-07 combination coverage). |
| 3 | Regression guards: AOSS OCU; catalog cells; cost CLI; OVH node-pool DependsOn; subnet min/cap/cap+1 for OVH/AWS/GCP; Scaleway N/A; 6KiB IAM quota present | ✓ VERIFIED | Re-verified serially: search OCU, bootstrap IAM quotas, aws/gcp/ovh/scaleway network, ovh runtime DependsOn, stack `TestCatalogCellProjectionMatrix`, cli cost — all ok. No regression from 01-10. |
| 4 | Mutating commands against `TierExperimental` print tier-naming warning before Pulumi, keyed on tier not provider allowlist; certified targets silent | ✓ VERIFIED | `TestExperimentalTargetWarnsAtPlanStack`, `TestExperimentalWarningLeavesJSONStdoutParseable` pass. |
| 5 | Experimental targets cannot silently succeed: stub day-2 `ErrNotSupported` enumeration; deploy refuses/announces infra-only; no-op `AcquireLock` warns | ✓ VERIFIED | Day-2 stubs + deploy refuse/announce unchanged and still pass. **AcquireLock clause closed:** ovh/scaleway/eksops call `platform.WarnNoDIYLock` then return noop release. Message: `warning: DIY deployment lock was not taken for %s`. `TestAcquireLockWarnsNoDIYLockTaken` passes on all three packages. AST AcquireLock carve-outs removed. |

**Score:** 4/5 truths verified (0 present, behavior-unverified; 1 uncertain — hosted CI outcome)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | F-01-07-1 search-proxy DependsOn gap on nginx-fpm queue/deploy/cron | Post-Phase-1 / later fix | 01-07-SUMMARY + 01-10 left untouched; not a ROADMAP SC |

Criterion 1 / QUALITY-06 **is not deferred to a later roadmap phase** (Phase 2–8 SCs do not absorb “first green `make verify` on main”). It remains a Phase 1 HUMAN_GATE — externally blocked, not rescheduled.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ---------- | ------ | ------- |
| `.github/workflows/ci.yml` | Partitioned lint + go-verify | ✓ VERIFIED (shape) / outcome HUMAN_GATE | Exists, substantive, wired; green run missing |
| `.golangci.yml` | No concurrency/timeout workarounds | ✓ VERIFIED | Keys absent |
| `internal/lintcoverage/coverage_test.go` | Matrix union coverage | ✓ VERIFIED | Passes locally |
| `docs/lint-policy.md` | Partitioning + measurements | ⚠️ PARTIAL | Narrative present; deferred-ci placeholders remain |
| `internal/cloud/aws/runtime/*.go` (non-test) | Split ≤400 lines | ✓ VERIFIED | 7 files; max 226; `runtime.go` gone |
| `internal/cloud/aws/runtime/runtime_test.go` | Post-split behaviour | ✓ VERIFIED | Race suite pass; no intentional test edits per 01-10 |
| `internal/platform/ops.go` | `WarnNoDIYLock` | ✓ VERIFIED | Stable message shape |
| `internal/cloud/{ovh,scaleway}/stack/ops.go` + eksops | Warn then noop | ✓ VERIFIED | Wired + tests |
| Prior QUALITY/TRUST artifacts (01-03…01-09) | Regression guards | ✓ VERIFIED | Spot-check pass (see truths 3–4) |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `planStack` | `warnExperimentalTarget` | call before backend | ✓ WIRED | Unchanged |
| deploy path | infra-only refuse/notice | `ErrNotSupported` / nil steps | ✓ WIRED | Unchanged |
| `result` job | `cache-prime`, `lint`, `go-verify` | `needs:` | ✓ WIRED | soft-fail only `cache-prime` |
| lintcoverage test | `ci.yml` matrix | parse + `go list` | ✓ WIRED | Guard passes |
| ovh/scaleway/eksops `AcquireLock` | `platform.WarnNoDIYLock` | stderr via `diyLockWarnOut` | ✓ WIRED | Was NOT_WIRED; closed by 01-10 |
| AST stub guard | unsupported receivers only | walk narrowed | ✓ WIRED | AcquireLock carve-out gone; warning tests own honesty |
| runtime_test.go | split package files | same package unexported access | ✓ WIRED | Intra-package; suite compiles and passes |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| AcquireLock warning | `planned.TargetDescriptor().ID` | PlannedStack | Real target id (e.g. ovh.mks) | ✓ FLOWING |
| Experimental warning | `CertificationTier()` | module registration | Real tier enum | ✓ FLOWING |
| Runtime construction | Args → containers/sidecars | Pulumi component `New` | Real graph from args | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------- |
| Runtime race suite post-split | `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/runtime/ -count=1` | ok | ✓ PASS |
| Line cap + no runtime.go | python wc assert | max 226; runtime.go absent | ✓ PASS |
| AcquireLock warn (ovh/scaleway/eksops) | `go test … -run TestAcquireLockWarnsNoDIYLockTaken` | ok ×3 | ✓ PASS |
| Lint matrix coverage | `go test ./internal/lintcoverage/ -count=1` | ok | ✓ PASS |
| Cost + tier + deploy CLI | `go test ./internal/cli/ -run 'TestCost\|TestExperimental\|TestDeploy…'` | ok | ✓ PASS |
| QUALITY network/OCU/IAM/OVH sample | serial package tests | ok | ✓ PASS |
| Catalog projection | `TestCatalogCellProjectionMatrix` | ok | ✓ PASS |
| gofmt drift | `gofmt -l` on prior + 01-10 files | empty | ✓ PASS |
| Hosted CI green | (not run — minutes) | deferred-ci | ? SKIP → human |
| Full `go test -race ./...` | (not run — 16GB/Cursor) | deferred-local | ? SKIP |
| A1 lint issue-set parity | (not run) | deferred | ? SKIP → human |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| — | — | No phase-declared `scripts/*/tests/probe-*.sh` | SKIPPED |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| QUALITY-01 | 01-03 | cost CLI tests | ✓ SATISFIED | spot-check |
| QUALITY-02 | 01-04 | IAM size quotas | ✓ SATISFIED | spot-check |
| QUALITY-03 | 01-04 | AOSS OCU table | ✓ SATISFIED | spot-check |
| QUALITY-04 | 01-07 | catalog × runtime matrices | ✓ SATISFIED | projection + runtime suite |
| QUALITY-05 | 01-05, 01-06 | subnet boundaries | ✓ SATISFIED | spot-check |
| QUALITY-06 | 01-01, 01-02 | CI lint partitioning actually green | ? NEEDS HUMAN | Shape + local guard ok; hosted green + A1 open |
| QUALITY-07 | 01-10 | split runtime.go | ✓ SATISFIED | Was ORPHANED; closed by 01-10 |
| QUALITY-08 | 01-06 | OVH regression tests | ✓ SATISFIED | spot-check |
| TRUST-01 | 01-09 | experimental tier warning | ✓ SATISFIED | spot-check |
| TRUST-02 | 01-08, 01-09, 01-10 | no silent success | ✓ SATISFIED | Stubs + deploy + AcquireLock warn |

**Note:** REQUIREMENTS.md still marks QUALITY-06 Complete — premature until a green hosted run is recorded. Treat as doc honesty debt under HUMAN_GATE, not as proof.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `docs/lint-policy.md` | 65–79 | `deferred-ci` / `deferred-local` placeholders | ⚠️ Warning | Honest offline markers; replace when minutes return |
| — | — | Prior AcquireLock silent success / 991-line runtime.go | ✓ Resolved | Closed by 01-10 |

No `TBD`/`FIXME`/`XXX` debt markers in 01-10 production files.

### Human Verification Required

### 1. Hosted force-all CI (when minutes return)

**Test:** `gh workflow run ci.yml --ref main -f all=true`; wait for conclusion; paste URL + per-target table into `docs/lint-policy.md`; clear QUALITY-06 deferred markers in REQUIREMENTS if appropriate.
**Expected:** Six `lint (…)` success; `go-verify` reaches `go test -race ./...`, govulncheck, licenses; php/docs/workflow-lint green.
**Why human:** Must not burn Actions from this verification; billing gate is external.

### 2. A1 lint parity (optional offline)

**Test:** Serial local golangci whole-module vs six partitions; diff issue sets.
**Expected:** Identical findings (or documented exceptions).
**Why human:** Heavy golangci unsafe under Cursor on 16GB Mac.

### Gaps Summary

**Re-verification after 01-10:** previous score 2/5 → **4/5**.

| Prior gap | Result |
| --------- | ------ |
| QUALITY-07 / criterion 2 (991-line `runtime.go`) | ✓ Closed |
| TRUST-02 AcquireLock clause / criterion 5 | ✓ Closed |
| gofmt drift | ✓ Closed |
| QUALITY-06 / criterion 1 hosted green | Open — HUMAN_GATE only |

No offline code gaps remain that later phases fail to absorb. Criterion 1 stays a Phase 1 contract item awaiting external CI minutes — escalate, do not re-plan.

---

_Verified: 2026-07-28T14:25:50Z_
_Verifier: Claude (gsd-verifier)_
