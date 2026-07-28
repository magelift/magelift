---
phase: 01-publishable-baseline-honest-fallbacks
plan: 04
subsystem: testing
tags: [iam, aoss, quality-02, quality-03, quotas, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Lint coverage guard + offline CI deferral (01-02)
provides:
  - Nine IAM policy documents size-asserted at quota and 90% margin
  - Serverless OCU accept/reject table pinning MageLift's empirical rule
  - Knowledge-bundle provenance that the OCU step is unpublished by AWS
affects: [QUALITY-02, QUALITY-03, TRUST-04, Phase 3 capability-matrix]

tech-stack:
  added: []
  patterns:
    - "IAM quota tests name each document and fail at 90% with a remedy message"
    - "AOSS OCU validator provenance recorded in code comments and knowledge lessons"

key-files:
  created:
    - docs/knowledge/lessons/AOSS collection-group OCU rule is undocumented by AWS.md
  modified:
    - internal/cloud/aws/bootstrap/identity_test.go
    - internal/cloud/aws/bootstrap/policy.go
    - internal/cloud/aws/search/search.go
    - internal/cloud/aws/search/search_test.go
    - docs/knowledge/lessons/AOSS collection-group OCU values cannot be zero.md
    - docs/knowledge/lessons/index.md
    - docs/knowledge/index.md
    - docs/knowledge/log.md

key-decisions:
  - "Compacted CI Resource:* statements (no permission change) so inline cleared 90% of 10240"
  - "Did not enforce 1700 OCU ceiling — marker only until next paid AWS pass"
  - "OCU table asserts validServerlessOCU false directly, not caller errors"

patterns-established:
  - "Quota subtests self-name the document so size regressions identify themselves"
  - "Empirical AWS rules carry provenance comments forbidding docs-driven relaxation"

requirements-completed: [QUALITY-02, QUALITY-03]

coverage:
  - id: D1
    description: All nine rendered IAM documents guarded at hard quota and 90% margin
    requirement: QUALITY-02
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/bootstrap/ -run Quota -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: Serverless OCU and capacity-range accept/reject tables pin MageLift's rule
    requirement: QUALITY-03
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/search/ -run 'OCU|CapacityRange' -count=1"
        status: pass
    human_judgment: false
  - id: D3
    description: Knowledge note records unpublished OCU step provenance (b8b957e)
    requirement: QUALITY-03
    verification:
      - kind: other
        ref: "docs/knowledge/lessons/AOSS collection-group OCU rule is undocumented by AWS.md"
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 04: IAM Quotas & OCU Provenance Summary

**Nine IAM documents size-guarded at their real API quotas (with 90% early fail), plus an OCU accept/reject table that pins MageLift's unpublished empirical step rule.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-07-28T11:38:09Z
- **Completed:** 2026-07-28T11:43:53Z
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- Relocated size assertions out of the least-privilege test into `TestIdentityPolicyDocumentsStayUnderIAMQuotas` with nine named subtests
- Locked `validServerlessOCU` / `validCapacityRange` with accept-and-reject tables and provenance comments (b8b957e, unenforced 1700 ceiling)
- Recorded the unpublished-step honesty finding in the knowledge bundle for TRUST-04 / capability-matrix follow-through

## Document sizes at plan close

| Document | Size | Quota | % |
|----------|------|-------|---|
| CI permissions boundary | 337 | 6144 | 5.5 |
| state permissions boundary | 710 | 6144 | 11.6 |
| build permissions boundary | 847 | 6144 | 13.8 |
| CI inline role policy | 9016 | 10240 | 88.0 |
| state inline role policy | 710 | 10240 | 6.9 |
| build inline role policy | 847 | 10240 | 8.3 |
| CI role trust policy | 386 | 2048 | 18.8 |
| state role trust policy | 386 | 2048 | 18.8 |
| build role trust policy | 386 | 2048 | 18.8 |

CI inline is the growth hotspot (88% after compaction). Boundaries and trusts have large headroom.

## Task Commits

1. **Task 1: Size-guard all nine rendered IAM policy documents** - `cbd9c21` (test)
2. **Task 2: Lock the serverless OCU validator with accept-and-reject table** - `ca8ab3b` (test)
3. **Task 3: Record the OCU provenance in the knowledge bundle** - `791f0b0` + `a8e1ec9` (docs)

**Plan metadata:** (pending docs commit)

## Files Created/Modified

- `internal/cloud/aws/bootstrap/identity_test.go` - Dedicated quota table; size asserts removed from least-privilege test
- `internal/cloud/aws/bootstrap/policy.go` - Collapsed CI `Resource:"*"` statements (permissions unchanged)
- `internal/cloud/aws/search/search.go` - Provenance + 1700 ceiling marker on `validServerlessOCU`
- `internal/cloud/aws/search/search_test.go` - `TestValidServerlessOCU`, `TestValidCapacityRange`
- `docs/knowledge/lessons/AOSS collection-group OCU rule is undocumented by AWS.md` - Honesty record
- Knowledge index/log cross-links

## Decisions Made

- Compact CI inline JSON structure rather than drop permissions — plan assumed under 90%; CI was at 94%
- Leave 1700 OCU unenforced with an explicit upgrade-path marker
- Flag `docs/capability-matrix.md` for Phase 3 TRUST-04 unverifiable-offline row (not edited this plan)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] CI inline already over 90% of PutRolePolicy quota**
- **Found during:** Task 1 verify
- **Issue:** Plan said documents were already under size; CI inline was 9646/10240 (94%), so the new 90% guard failed immediately
- **Fix:** Collapse equivalent `Resource:"*"` Allow statements into one Action list (no permission change) → 9016 (88%)
- **Files modified:** `internal/cloud/aws/bootstrap/policy.go`
- **Verification:** Quota + LeastPrivilege tests pass with `-race`
- **Committed in:** `cbd9c21`

**2. [Rule 3 - Blocking] OKF rebuild tools missing PyYAML**
- **Found during:** Task 3 housekeeping
- **Issue:** `rebuild-index.py` / `validate-okf.py` fail with `No module named 'yaml'` (PEP 668)
- **Fix:** Manually insert lesson into `lessons/index.md` and bump count to 241 in `docs/knowledge/index.md`
- **Files modified:** index files
- **Verification:** grep finds note + `b8b957e`
- **Committed in:** `a8e1ec9`

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** QUALITY-02/03 delivered; CI inline now measurable under the 90% gate.

## Issues Encountered

- PyYAML unavailable in system Python — index regeneration is manual until toolchain provides it

## Known Stubs

None.

## Forward flags

- Phase 3 / TRUST-04: add unverifiable-offline capability-matrix row for AOSS collection-group OCU step rule

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 01-04 complete offline; next plan **01-05**
- QUALITY-02 and QUALITY-03 closed for this milestone slice

## Self-Check: PASSED

- FOUND: `internal/cloud/aws/bootstrap/identity_test.go` (Quota test)
- FOUND: `internal/cloud/aws/search/search_test.go` (OCU/CapacityRange)
- FOUND: `docs/knowledge/lessons/AOSS collection-group OCU rule is undocumented by AWS.md`
- FOUND: commits `cbd9c21`, `ca8ab3b`, `791f0b0`, `a8e1ec9`

---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*
