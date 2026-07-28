---
phase: 02-tag-ready-release-surface
verified: 2026-07-28T15:43:00Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification: false
gaps: []
deferred: []
---

# Phase 2: Tag-Ready Release Surface Verification Report

**Phase Goal:** `v1.0.0-rc.1` becomes taggable — the version story is consistent everywhere, the contract carries an explicit RC stability statement, the packaging gate is closed, and a stranger can go from clone to green.

**Verified:** 2026-07-28T15:43:00Z
**Status:** passed
**Re-verification:** No prior `02-VERIFICATION.md` on disk — initial verification after formerly-human gates (RELEASE-04 packaging smoke, RELEASE-06 fresh-worktree `make verify`) were closed by agent with durable evidence.

## Recommendation

**Mark Phase 2 complete and start Phase 3.** All five ROADMAP success criteria (RELEASE-01, 02, 03, 04, 06) are observably true in the repo and in recorded proofs. Hosted GitHub Actions / QUALITY-06 remains a **Phase 1** HUMAN_GATE only — it is not a Phase 2 contract item and was not required for this pass. Do not burn Actions minutes for Phase 2.

Housekeeping (non-blocking): refresh stale `02-02-SUMMARY.md` (still narrates `VERIFY_PROOF_BLOCKED`) and close WINDOWS.md unmet-truth #8 so planning docs match `scratch/02-02-verify-proof.txt`.

## Goal Achievement

### Observable Truths

Roadmap success criteria are the contract. Plan must_haves add detail but do not shrink this list.

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | `README.md`, `docs/versioning.md`, and `docs/release-readiness.md` each name `v1.0.0-rc.1` as the first public tag; no remaining sentence describes the project as pre-alpha or `v0.x` (CHANGELOG history excepted) | ✓ VERIFIED | All three name `v1.0.0-rc.1`. Negative scan for `pre-alpha` / `v0.x` / `v0.` product-status literals: clean on all three. Contract freeze row labeled `v1.0.0-rc.1`. Satellites: SECURITY.md + CHANGELOG `[Unreleased]` aligned; SUPPORT.md status strip removed (purpose-only). |
| 2 | `docs/versioning.md` carries a stability statement for `sdk/v1` and `platform.StackModule` that names what may change during RC — explicitly reserving Phase 6 shared-Kubernetes port changes | ✓ VERIFIED | Section `RC stability statement (sdk/v1 and platform.StackModule)` locks Target/Capability/Hook + StackModule contract; **May change** lists experimental cells and kube day-2 ports; **Phase 6 reservation** names Observe + `deploy.Steps` → `internal/cloud/kube` for EKS/GKE/MKS/Kapsule without a `v2` wait. |
| 3 | `make release-smoke` completes (serial, single-target); Packaging smoke moves Partial → Closed with run date and output recorded | ✓ VERIFIED | Board row **Closed** with `2026-07-28T15:11:04Z` and `release smoke ok binary=dist/magelift_darwin_arm64_v8.0/magelift (serial single-target)`. Log: `scratch/02-04-release-smoke.log` (`goreleaser check` + `--single-target --parallelism=1`). Script enforces serial flags. No `dist/` committed. Maintainer allowed agent serial smoke under Cursor with abort-on-pressure (plan preferred plain Terminal; outcome evidence is sufficient). |
| 4 | Fresh checkout following only `CONTRIBUTING.md` reaches green `make verify` | ✓ VERIFIED | `scratch/02-02-verify-proof.txt`: `VERIFY_PROOF_OK`, `proof_type: git_worktree`, path `/tmp/magelift-verify-proof-final`, commit `ad27f711…`, `utc: 2026-07-28T15:32:54Z`, `make_verify: exit 0`. Worktree still present at that commit with `build/vendor` (composer install per CONTRIBUTING). CONTRIBUTING lists Go/PHP 8.2+/Composer/MkDocs; honest hosted-CI deferral (no "CI green on main"). Host now has PHP 8.5.8 + Composer 2.10.2. Full `make verify` not re-run by verifier (prior proof + worktree integrity). |
| 5 | `examples/custom-cli` builds and registers an out-of-tree provider following `docs/adding-a-provider.md` alone, from a clean module cache | ✓ VERIFIED | Docs + example README carry matching clean-`GOMODCACHE`/`GOCACHE` recipes and `internal/` honesty. `main.go` registers `stubModule` via `RegisterModule`; Plan/Program refuse with `ErrNotSupported`. Spot-check: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./examples/custom-cli/ -run TestStub` → ok (~2s). Empty-cache re-build aborted mid-download when swap ~9 GB used (AGENTS.md); prior 02-03 SUMMARY clean-cache `version: dev` / `BUILD_EXIT=0` stands as the empty-cache proof. |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Deferred Items

None for Phase 2 roadmap SCs. Hosted CI / QUALITY-06 remains Phase 1 HUMAN_GATE (not absorbed by Phase 3–8 success criteria as a Phase 2 substitute).

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ----------- | ------ | ------- |
| `README.md` | RC.1 first-public-tag banner | ✓ VERIFIED | Lines 7–9 |
| `docs/versioning.md` | RC freeze + stability + Phase 6 reservation | ✓ VERIFIED | Substantive; wired via README + release-readiness links |
| `docs/release-readiness.md` | Contract freeze + Packaging smoke Closed | ✓ VERIFIED | Both Closed with evidence |
| `CONTRIBUTING.md` | Full verify prerequisites + honest local-vs-hosted CI | ✓ VERIFIED | Prerequisites + Testing deferral paragraph |
| `docs/adding-a-provider.md` | Clean-cache recipe + internal/ honesty | ✓ VERIFIED | Community binary + RELEASE-03 section |
| `examples/custom-cli/*` | Registration demo + stub + tests | ✓ VERIFIED | `main.go`, `stub_module.go`, `stub_module_test.go`, README |
| `scripts/release-smoke-local.sh` / `make release-smoke` | Serial single-target smoke | ✓ VERIFIED | Flags locked; Makefile target present |
| `scratch/02-02-verify-proof.txt` | VERIFY_PROOF_OK record | ✓ VERIFIED | Matches live worktree HEAD |
| `scratch/02-04-release-smoke.log` | release smoke ok line | ✓ VERIFIED | Matches board evidence |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| README status banner | `docs/versioning.md` | Link in banner | ✓ WIRED | Points to RC freeze surface |
| `docs/versioning.md` Phase 6 reservation | ROADMAP Phase 6 kube ports | Explicit Observe/`deploy.Steps` text | ✓ WIRED | Names consolidation into `internal/cloud/kube` |
| `docs/release-readiness.md` Packaging smoke | `make release-smoke` / smoke log | Closed row + Packaging smoke record | ✓ WIRED | Date + exact ok line |
| CONTRIBUTING Prerequisites | `make verify` chain | Documented tools → Makefile `verify` | ✓ WIRED | `verify: generate-check … php-test docs workflow-check` |
| `docs/adding-a-provider.md` | `examples/custom-cli` | Clean-cache recipe + RegisterModule | ✓ WIRED | Matching commands; stub registered in `main` |

### Data-Flow Trace (Level 4)

N/A for dynamic UI data. Release/docs phase: evidence flows from runnable commands → scratch logs / board rows (Packaging smoke, verify proof) rather than hardcoded empty collections.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Stub registration + Plan refusal | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./examples/custom-cli/ -count=1 -run TestStub` | `ok` ~2.1s | ✓ PASS |
| Packaging smoke evidence present | `rg 'release smoke ok' scratch/02-04-release-smoke.log` | Match | ✓ PASS |
| Fresh-verify proof + worktree | Read proof + `git -C /tmp/magelift-verify-proof-final rev-parse HEAD` | `VERIFY_PROOF_OK`; HEAD=`ad27f711…`; vendor present | ✓ PASS |
| Empty-cache custom-cli rebuild | Empty `GOMODCACHE`/`GOCACHE` `go test`/`go build` | Aborted — swap ~9 GB used | ? SKIP (prior 02-03 proof retained) |
| Full `make verify` re-run | — | Not re-run (memory; proof already recorded) | ? SKIP (proof file + worktree) |
| Hosted Actions force-all | — | Explicitly out of Phase 2 scope | ? SKIP |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| — | — | No phase-declared `scripts/*/tests/probe-*.sh` | SKIPPED |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| RELEASE-01 | 02-01 | Version story consistent; first tag `v1.0.0-rc.1` | ✓ SATISFIED | Truth 1 |
| RELEASE-02 | 02-01 | `sdk/v1` + `StackModule` RC stability statement | ✓ SATISFIED | Truth 2 |
| RELEASE-03 | 02-03 | Out-of-tree provider via adding-a-provider + custom-cli | ✓ SATISFIED | Truth 5 |
| RELEASE-04 | 02-04 | `make release-smoke` closes Packaging smoke | ✓ SATISFIED | Truth 3 |
| RELEASE-06 | 02-02 | Clone → green `make verify` via CONTRIBUTING | ✓ SATISFIED | Truth 4 |

No orphaned Phase 2 requirements. RELEASE-05 is Phase 8 (gate-board audit on tag day) — correctly out of scope.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `examples/custom-cli/stub_module.go` | Plan/Program | Intentional `ErrNotSupported` stub | ℹ️ Info | Demo refusal — documented; not a fake deployable |
| `02-02-SUMMARY.md` | frontmatter/body | Stale `VERIFY_PROOF_BLOCKED` narrative | ⚠️ Warning | Docs drift vs `scratch/02-02-verify-proof.txt` `VERIFY_PROOF_OK` — refresh SUMMARY |
| `.planning/WINDOWS.md` | unmet-truth #8 | Still `open` for PHP/Composer blocker | ⚠️ Warning | Planning hygiene; RELEASE-06 outcome is closed in scratch proof |

No `TBD`/`FIXME`/`XXX` debt markers in phase-touched product files (mktemp `XXXXXX` paths are not debt).

### Human Verification Required

None for Phase 2. Formerly-human gates are closed with recorded evidence:

- Packaging smoke: Closed + log
- Fresh `make verify`: `VERIFY_PROOF_OK` + live worktree

Phase 1 hosted CI remains a separate HUMAN_GATE (not listed here as a Phase 2 blocker).

### Gaps Summary

No actionable Phase 2 gaps. Goal achieved: version story + RC contract + packaging Closed + contributor verify path + custom-cli registration surface are all present, substantive, and evidenced.

---

_Verified: 2026-07-28T15:43:00Z_
_Verifier: Claude (gsd-verifier)_
