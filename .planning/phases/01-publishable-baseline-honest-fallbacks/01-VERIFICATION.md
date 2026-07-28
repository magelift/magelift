---
phase: 01-publishable-baseline-honest-fallbacks
verified: 2026-07-28T14:00:52Z
status: gaps_found
score: 2/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "The go job passes on main — genuinely, not by being skipped — with partitioned lint and every make verify target green in CI"
    status: failed
    reason: "Hosted CI green proof never obtained (Actions minutes exhausted 2026-07-28). Workflow shape exists on disk; no durable green run URL; force-all eight-target table and A1 partition-vs-whole-module lint parity never recorded. Two Go files currently fail gofmt -l (struct-field alignment only)."
    artifacts:
      - path: ".github/workflows/ci.yml"
        issue: "cache-prime / lint×6 / go-verify present and wired into result.needs, but criterion requires a successful main run, not file presence"
      - path: "docs/lint-policy.md"
        issue: "deferred-ci placeholders for force-all URL, race wall-time, and CI go-verify; A1 issue-set parity evidence absent"
      - path: "internal/cli/lifecycle_test.go"
        issue: "gofmt -l reports drift (field alignment)"
      - path: "internal/cloud/aws/stack/component_test.go"
        issue: "gofmt -l reports drift (field alignment)"
    missing:
      - "When Actions minutes return: gh workflow run ci.yml --ref main -f all=true; record run URL + per-target table in docs/lint-policy.md"
      - "Record partitioned vs whole-module golangci issue-set parity (research assumption A1)"
      - "gofmt -w the two drifted test files so fmt-check can pass"
  - truth: "No non-test file under internal/cloud/aws/runtime/ exceeds 400 lines; runtime_test.go suite proves the split preserved behaviour"
    status: failed
    reason: "QUALITY-07 never planned or executed. Plan 01-10 was referenced across 01-01/01-07/01-08/01-09 but does not exist. runtime.go is still 991 lines — the exact pre-phase size."
    artifacts:
      - path: "internal/cloud/aws/runtime/runtime.go"
        issue: "991 lines; sole non-test file in package; exceeds 400-line cap"
    missing:
      - "Create and execute plan 01-10 (or equivalent): behaviour-preserving split of runtime.go; re-run runtime_test.go baseline including TestRuntimeAddsSigV4ProxyForMagentoOpenSearch"
  - truth: "No-op AcquireLock implementations warn that no lock was taken rather than returning a release function that implies one was"
    status: failed
    reason: "Criterion 5's third clause. ovh/scaleway/eksops AcquireLock still return (noopRelease, nil) with no warning. AST guards explicitly carve AcquireLock out as 'plan 01-10 revisits this exception'. CLI AcquireLock call site in root.go does not warn."
    artifacts:
      - path: "internal/cloud/ovh/stack/ops.go"
        issue: "AcquireLock returns working release, nil error, no warn"
      - path: "internal/cloud/scaleway/stack/ops.go"
        issue: "Same silent no-op"
      - path: "internal/cloud/aws/eksops/ops.go"
        issue: "Same silent no-op (experimental AWS K8s path)"
      - path: "internal/cloud/ovh/stack/ops_test.go"
        issue: "Named AcquireLock exception documents unfinished 01-10 work"
    missing:
      - "Warn (stderr) that no DIY lock was taken, or return ErrNotSupported; drop AST carve-out; cover with a CLI/ops test"
deferred:
  - truth: "F-01-07-1 search-proxy DependsOn skipped for nginx-fpm non-php-fpm task containers"
    addressed_in: "Post-01-10 / later correctness fix (not a ROADMAP Phase 1 success criterion)"
    evidence: "01-07-SUMMARY Finding F-01-07-1 — recorded only so 01-10 split baseline stays pure; Phase 6 shared Kubernetes day-2 may absorb related DependsOn harness work"
human_verification:
  - test: "When GitHub Actions minutes return, dispatch force-all CI on main and confirm six lint partitions + go-verify + php/docs/workflow-lint all succeed"
    expected: "Durable green run URL; every make verify target executes; docs/lint-policy.md placeholders replaced with real numbers"
    why_human: "Verifier must not burn Actions minutes; hosted outcome cannot be proven offline"
  - test: "Optionally run serial whole-module vs partitioned golangci issue-set diff (A1) in a plain Terminal outside Cursor"
    expected: "Identical issue sets (or documented intentional differences)"
    why_human: "Full golangci on 16GB Mac under Cursor risks OOM/kernel panic per AGENTS.md"
---

# Phase 1: Publishable Baseline & Honest Fallbacks Verification Report

**Phase Goal:** The repository survives a stranger's first read — CI actually runs and passes the verification suite for the first time, the highest-risk file is no longer a single 991-line construction path, every recently fixed bug has a regression guard, and no target can silently pretend to support something it does not.

**Verified:** 2026-07-28T14:00:52Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Roadmap success criteria are the contract. Plan must_haves add detail but cannot shrink this list.

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | `go` job passes on main (partitioned lint + all `make verify` targets in CI); A1 partitioning parity verified | ✗ FAILED | Workflow committed (`.github/workflows/ci.yml`: `cache-prime`, lint matrix aws/gcp/ovh/scaleway/core/aggregate, `go-verify`, listed in `result.needs`). `.golangci.yml` has no `run.concurrency` / `run.timeout` keys. Union-coverage guard passes locally (`go test ./internal/lintcoverage/`). **No hosted green proof** — Actions minutes exhausted; force-all table and A1 issue-set diff deferred. `gofmt -l` currently reports 2 files. |
| 2 | No non-test `internal/cloud/aws/runtime/` file >400 lines; suite proves split preserved behaviour | ✗ FAILED | `wc -l` → `runtime.go` **991** lines. No plan 01-10. QUALITY-07 unchecked in REQUIREMENTS.md and claimed by zero PLAN frontmatter `requirements:` lists. Combination tests from 01-07 exist as a split *baseline*, not as post-split proof. |
| 3 | Regression guards: AOSS OCU; catalog cells; cost CLI; OVH node-pool DependsOn; subnet min/cap/cap+1 for OVH/AWS/GCP; Scaleway N/A; 6KiB IAM quota present | ✓ VERIFIED | Spot-checked serially (`GOMAXPROCS=1 GOFLAGS=-p=1`): search OCU, bootstrap IAM quotas, aws/gcp/ovh network boundaries, ovh runtime DependsOn, scaleway single-range, stack `TestCatalogCellProjectionMatrix`, runtime container-graph + SigV4, cli cost/tier/deploy tests — all ok. Full-module `go test -race ./...` remains deferred-local/deferred-ci (environment), not absence of guards. |
| 4 | Mutating commands against `TierExperimental` print tier-naming warning before Pulumi, keyed on tier not provider allowlist; certified targets silent | ✓ VERIFIED | `warnExperimentalTarget` in `planStack` (`lifecycle.go`); keyed on `CertificationTier() == TierExperimental`. Tests: `TestExperimentalTargetWarnsAtPlanStack` (includes experimental aws eks), `TestExperimentalWarningLeavesJSONStdoutParseable` — passed. |
| 5 | Experimental targets cannot silently succeed: stub day-2 `ErrNotSupported` enumeration; deploy refuses/announces infra-only; no-op `AcquireLock` warns | ✗ FAILED | Day-2 stubs + AST sentinel guards verified (`ovh/stack`, `scaleway/stack` tests ok). Deploy refuse/announce verified (`TestDeployRefusesInfraOnlyWithoutFlag`, `TestDeployInfraOnlyFlagAnnouncesSkip`). **`AcquireLock` still silent no-op** on ovh/scaleway/eksops — clause fails; owned by never-executed 01-10. |

**Score:** 2/5 truths verified (0 present, behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | F-01-07-1 search-proxy DependsOn gap on nginx-fpm queue/deploy/cron | Post-split / later fix | 01-07-SUMMARY finding; not a ROADMAP SC; do not treat as Phase 1 blocker |

Items intentionally **not** deferred to later roadmap phases (remain gaps):

- Hosted CI green + force-all table + A1 — Phase 2 *depends on* Phase 1 making `make verify` true; Phase 2 SCs do not absorb this.
- QUALITY-07 runtime split — not in Phases 2–8 success criteria.
- AcquireLock warn — Phase 6 covers real OVH/Scaleway state locks, not the experimental no-op warning clause of Phase 1 SC5.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ---------- | ------ | ------- |
| `.github/workflows/ci.yml` | Partitioned lint + go-verify | ✓ VERIFIED (shape) / ✗ unmet outcome | Exists, substantive, wired; green run missing |
| `.golangci.yml` | No concurrency/timeout workarounds | ✓ VERIFIED | Keys absent; comment only |
| `internal/lintcoverage/coverage_test.go` | Matrix union coverage | ✓ VERIFIED | Passes locally |
| `docs/lint-policy.md` | Partitioning + measurements | ⚠️ PARTIAL | Narrative present; deferred-ci placeholders; no A1 parity |
| `internal/cli/cost_test.go` | QUALITY-01 | ✓ VERIFIED | Tests pass |
| `internal/cloud/aws/bootstrap/identity_test.go` | IAM quota + 6144 | ✓ VERIFIED | `TestIdentityPolicyDocumentsStayUnderIAMQuotas` |
| `internal/cloud/aws/search/search_test.go` | OCU table | ✓ VERIFIED | `TestValidServerlessOCU` |
| `internal/cloud/aws/network/network.go` + tests | AWS carve cap | ✓ VERIFIED | `validateCarveCapacity` + boundary tests |
| `internal/cloud/gcp/network/network.go` + `network_test.go` | GCP index cap | ✓ VERIFIED | First test file; min/max/max+1 |
| `internal/cloud/ovh/runtime/runtime_test.go` | Node-pool DependsOn | ✓ VERIFIED | `TestMKSNodePoolDependencyOrdering` |
| `internal/cloud/ovh/network/network_test.go` | Index 0/15/16 | ✓ VERIFIED | Spot-check pass |
| `internal/cloud/scaleway/network/network_test.go` | Single PN range | ✓ VERIFIED | `TestNewKeepsSinglePrivateNetworkRange` |
| `internal/cloud/aws/stack/component_test.go` | Projection matrix | ✓ VERIFIED | Pass; gofmt drift |
| `internal/cloud/aws/runtime/runtime_test.go` | Container-graph + SigV4 | ✓ VERIFIED | Spot-check pass (baseline for missing split) |
| `internal/cloud/aws/runtime/runtime.go` | Split ≤400 lines | ✗ FAILED | Still 991 lines |
| `internal/cloud/ovh/stack/ops_test.go` / scaleway twin | 15× stub enumeration + AST guard | ✓ VERIFIED | Pass; AcquireLock exception documented |
| `internal/cli/lifecycle.go` + `lifecycle_test.go` | Tier warn + deploy fallback | ✓ VERIFIED (partial vs SC5) | Tier + deploy ok; lock warn missing |
| `internal/cli/ports.go` + `ports_test.go` | Tier in not-supported messages | ✓ VERIFIED | Pass |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `planStack` | `warnExperimentalTarget` | call before backend | ✓ WIRED | `lifecycle.go:286` |
| deploy path | infra-only refuse/notice | `ErrNotSupported` / nil steps | ✓ WIRED | refuse without flag; notice with `--infra-only` |
| `result` job | `cache-prime`, `lint`, `go-verify` | `needs:` | ✓ WIRED | soft-fail only `cache-prime` |
| lintcoverage test | `ci.yml` matrix | parse + `go list` | ✓ WIRED | Guard fails on gaps/overlaps |
| AST stub guard | `AcquireLock` | named exception | ⚠️ PARTIAL | Intentionally unwired to warn path — 01-10 debt |
| CLI lock | experimental `AcquireLock` | `root.go:163` | ✗ NOT_WIRED to warn | Returns release that implies lock taken |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Experimental warning | `planned.CertificationTier()` | module registration / stub planned | Real tier enum | ✓ FLOWING |
| not-supported message | provider/runtime/tier/surface | `ports.notSupportedMessage` | Real strings | ✓ FLOWING |
| N/A for docs/CI config artifacts | — | — | — | skipped |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Lint matrix coverage | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/lintcoverage/ -count=1` | ok | ✓ PASS |
| Cost + tier + deploy CLI | `go test ./internal/cli/ -count=1 -run 'TestCost\|TestExperimental\|TestDeployRefuse\|TestDeployInfra'` | ok | ✓ PASS |
| OCU + IAM quotas | `go test ./internal/cloud/aws/{search,bootstrap}/ …` | ok | ✓ PASS |
| Network caps OVH/AWS/GCP/Scaleway | serial package tests | ok | ✓ PASS |
| OVH runtime DependsOn | `go test ./internal/cloud/ovh/runtime/` | ok | ✓ PASS |
| Stub ops enumeration | `go test ./internal/cloud/{ovh,scaleway}/stack/` | ok | ✓ PASS |
| Catalog + runtime matrices | stack + runtime named runs | ok | ✓ PASS |
| Hosted CI green | (not run — minutes) | deferred-ci | ? SKIP |
| Full `go test -race ./...` | (not run — 16GB/Cursor) | deferred-local | ? SKIP |
| A1 lint issue-set parity | (not run) | deferred | ? SKIP |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| — | — | No phase-declared `scripts/*/tests/probe-*.sh` | SKIPPED |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| QUALITY-01 | 01-03 | cost CLI tests | ✓ SATISFIED | `cost_test.go` + spot-check |
| QUALITY-02 | 01-04 | IAM size quotas | ✓ SATISFIED | `TestIdentityPolicyDocumentsStayUnderIAMQuotas` |
| QUALITY-03 | 01-04 | AOSS OCU table | ✓ SATISFIED | `TestValidServerlessOCU` |
| QUALITY-04 | 01-07 | catalog × runtime matrices | ✓ SATISFIED | projection + container-graph tests |
| QUALITY-05 | 01-05, 01-06 | subnet boundaries all providers | ✓ SATISFIED | AWS/GCP/OVH caps + Scaleway N/A test |
| QUALITY-06 | 01-01, 01-02 | CI lint partitioning actually green | ✗ BLOCKED | Workflow + local guard exist; hosted green + A1 open |
| QUALITY-07 | **ORPHANED** — no plan | split runtime.go | ✗ BLOCKED | Still 991 lines; 01-10 never written |
| QUALITY-08 | 01-06 | OVH regression tests | ✓ SATISFIED | network + runtime_test.go |
| TRUST-01 | 01-09 | experimental tier warning | ✓ SATISFIED | planStack warn + tests |
| TRUST-02 | 01-08, 01-09 | no silent success | ✗ BLOCKED (partial) | Stubs + deploy fixed; AcquireLock still silent — REQUIREMENTS.md marks Complete prematurely |

**Orphaned requirement:** QUALITY-07 mapped to Phase 1 in REQUIREMENTS.md / ROADMAP but appears in **zero** PLAN `requirements:` fields. ROADMAP still lists it under Phase 1 Requirements while claiming 9/9 plans executed.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `internal/cloud/ovh/stack/ops.go` | 68–70 | Silent success `AcquireLock` | 🛑 Blocker | Criterion 5 clause |
| `internal/cloud/scaleway/stack/ops.go` | 69–71 | Same | 🛑 Blocker | Criterion 5 clause |
| `internal/cloud/aws/eksops/ops.go` | 214–216 | Same | 🛑 Blocker | Experimental AWS K8s |
| `internal/cloud/aws/runtime/runtime.go` | — | 991-line construction path | 🛑 Blocker | Criterion 2 / QUALITY-07 |
| `docs/lint-policy.md` | 65–79 | `deferred-ci` / `deferred-local` placeholders | ⚠️ Warning | Honest offline markers, not stubs |
| `internal/cli/lifecycle_test.go` | ~396 | gofmt drift | ⚠️ Warning | Would fail CI `fmt-check` |
| `internal/cloud/aws/stack/component_test.go` | ~375 | gofmt drift | ⚠️ Warning | Would fail CI `fmt-check` |

No `TBD`/`FIXME`/`XXX` debt markers in phase production files scanned.

### Human Verification Required

### 1. Hosted force-all CI (when minutes return)

**Test:** `gh workflow run ci.yml --ref main -f all=true`; wait for conclusion; paste URL + per-target table into `docs/lint-policy.md`.
**Expected:** Six `lint (…)` success; `go-verify` reaches `go test -race ./...`, govulncheck, licenses; php/docs/workflow-lint green.
**Why human:** Must not burn Actions from this verification; billing gate is external.

### 2. A1 lint parity (optional offline)

**Test:** Serial local golangci whole-module vs six partitions; diff issue sets.
**Expected:** Identical findings (or documented exceptions).
**Why human:** Heavy golangci unsafe under Cursor on 16GB Mac.

### Gaps Summary

Phase 1 executed **9 of an implied 10 plans**. Offline plans 01-03…01-09 delivered the regression-guard and honesty work for criteria 3–4 and most of 5. Three roadmap must-haves remain open:

1. **Criterion 1 / QUALITY-06 outcome** — CI *shape* shipped; CI *green* deferred (minutes). Also A1 parity + current gofmt drift.
2. **Criterion 2 / QUALITY-07** — runtime split never planned (`01-10` missing); `runtime.go` still 991 lines.
3. **Criterion 5 AcquireLock clause / TRUST-02 remainder** — explicit 01-10 carry-over; still silent no-op.

**Advancing to Phase 2 with only “deferred CI” is not sufficient.** Phase 2 depends on Phase 1 for a green `make verify` story, but even granting a CI override, QUALITY-07 and AcquireLock are unfinished Phase 1 scope that later phases do not absorb.

Suggested override path (human only, if intentional): accept deferred hosted CI temporarily **after** 01-10 closes QUALITY-07 + AcquireLock, and keep a HUMAN_GATE until the force-all run lands — do not override QUALITY-07 or AcquireLock as “CI deferred.”

---

_Verified: 2026-07-28T14:00:52Z_
_Verifier: Claude (gsd-verifier)_
