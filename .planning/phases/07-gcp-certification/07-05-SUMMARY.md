---
phase: 07-gcp-certification
plan: 05
subsystem: infra
tags: [cloudflare, dns, cutover, migrate-04, shell]

requires:
  - phase: 06-shared-kubernetes-day-2
    provides: Cloudflare Zone.DNS Edit handoff + preferred preview host
provides:
  - Offline-testable Cloudflare DNS upsert/cleanup script for MIGRATE-04
  - Documented Zone.DNS Edit vs Wrangler OAuth cutover path
affects: [07-gcp-certification, harness cutover:dns cell]

tech-stack:
  added: [curl + bash Cloudflare API v4 client]
  patterns: [CURL_BIN mock for offline DNS CRUD, --dry-run without token]

key-files:
  created:
    - scripts/cutover-dns-cloudflare.sh
    - scripts/acceptance/cutover-dns-cloudflare_test.sh
  modified:
    - docs/migrating-from-paas.md
    - .planning/phases/06-shared-kubernetes-day-2/06-PHASE7-HANDOFF.md

key-decisions:
  - "Auth via CLOUDFLARE_API_TOKEN or CF_API_TOKEN Bearer only; dry-run skips token"
  - "Hostname TARGET → CNAME; IPv4 → A; TTL 120; proxied false unless --proxied"
  - "CURL_BIN override for acceptance mocks — no Go SDK, no live DNS in plan verify"

patterns-established:
  - "DNS cutover script: dry-run logs to stderr, JSON on stdout for parsers"
  - "Acceptance stub curl asserts method/URL/body without contacting api.cloudflare.com"

requirements-completed: [MIGRATE-04]

coverage:
  - id: D1
    description: "cutover-dns-cloudflare.sh upserts CNAME/A and supports --cleanup/--dry-run with Zone.DNS Edit token auth"
    requirement: MIGRATE-04
    verification:
      - kind: unit
        ref: "MAGELIFT_CUTOVER_HOST=magelift-preview.alexandrecourtiol.com TARGET=example.invalid ./scripts/cutover-dns-cloudflare.sh --dry-run"
        status: pass
    human_judgment: false
  - id: D2
    description: "Offline mocked curl test covers zone resolve, create, update, cleanup, and missing-token failure"
    requirement: MIGRATE-04
    verification:
      - kind: unit
        ref: "bash scripts/acceptance/cutover-dns-cloudflare_test.sh"
        status: pass
    human_judgment: false
  - id: D3
    description: "Docs forbid Wrangler OAuth and name preferred host magelift-preview.alexandrecourtiol.com"
    requirement: MIGRATE-04
    verification:
      - kind: other
        ref: "rg Zone.DNS|Wrangler|magelift-preview docs/migrating-from-paas.md"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-30
status: complete
---

# Phase 7 Plan 05: Cloudflare DNS Cutover Script Summary

**Shell Cloudflare API cutover tool for `magelift-preview.alexandrecourtiol.com` with dry-run and mocked-curl offline proof (no live DNS writes).**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-30T11:40:56Z
- **Completed:** 2026-07-30T11:43:39Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Delivered `scripts/cutover-dns-cloudflare.sh`: zone resolve (alexandrecourtiol.com → acourtiol.com fallback), CNAME/A upsert, `--cleanup`, `--dry-run`, `--proxied`
- Documented Zone.DNS Edit token requirement and Wrangler OAuth ban in `docs/migrating-from-paas.md`; linked Phase 6 handoff
- Green offline acceptance test with `CURL_BIN` stub (create → update → delete + missing-token gate)

## Task Commits

1. **Task 1: End-to-end Cloudflare DNS upsert + cleanup script** - `dd447b2` (feat)
2. **Task 2: Offline shell self-test with mocked curl** - `1a78bd1` (test)

**Plan metadata:** `19121bc` (docs: complete plan)

## Files Created/Modified

- `scripts/cutover-dns-cloudflare.sh` — Cloudflare DNS CRUD for cutover rehearsal
- `scripts/acceptance/cutover-dns-cloudflare_test.sh` — mocked curl self-test
- `docs/migrating-from-paas.md` — Cloudflare preview rehearsal section
- `.planning/phases/06-shared-kubernetes-day-2/06-PHASE7-HANDOFF.md` — script pointer

## Decisions Made

- Prefer `CLOUDFLARE_API_TOKEN` with `CF_API_TOKEN` alias; refuse mutating calls without token
- `--dry-run` prints intended API calls on stderr and synthesizes success JSON (no network)
- Record type inferred from TARGET (IPv4 → A, else CNAME); short TTL 120; orange-cloud off by default

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Dry-run zone path matched `/zones/{id}/dns_records`**
- **Found during:** Task 1 (tracer verify)
- **Issue:** `/zones?*` glob treated `/zones/dry-run-zone/...` as a zone list, returning a fake record id and taking the update path on create
- **Fix:** Match only `/zones` or `/zones?*` (literal `?`); send DRY-RUN logs to stderr so JSON parse stays clean
- **Files modified:** `scripts/cutover-dns-cloudflare.sh`
- **Verification:** dry-run prints `creating` not `updating`; mocked test green
- **Committed in:** `dd447b2`

**Total deviations:** 1 auto-fixed (Rule 1)
**Impact on plan:** Correctness fix only; no scope creep.

## Issues Encountered

None beyond the dry-run path glob (auto-fixed).

## User Setup Required

Live cutover (not required for this plan's verify) needs:

```sh
export CLOUDFLARE_API_TOKEN=...   # Zone.DNS Edit on alexandrecourtiol.com / acourtiol.com
export MAGELIFT_CUTOVER_HOST=magelift-preview.alexandrecourtiol.com
TARGET=<applicationURL-or-LB> ./scripts/cutover-dns-cloudflare.sh
./scripts/cutover-dns-cloudflare.sh --cleanup
```

## Next Phase Readiness

MIGRATE-04 DNS tooling ready for harness `cutover:dns` cell on the paid GCP pass. Remaining Phase 7 plans own live cells / certification honesty.

---
*Phase: 07-gcp-certification*
*Completed: 2026-07-30*

## Self-Check: PASSED
