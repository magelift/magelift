---
phase: 07-gcp-certification
plan: 04
subsystem: database
tags: [dumpimport, kubectl, cloud-sql, private-ip, migrate]

requires:
  - phase: 05-data-migration-cutover
    provides: dumpimport host/compose Import + nonempty/--yes gates + tiny.sql fixture
provides:
  - Kube-adjacent dumpimport runner (RunnerKube) for private-IP Cloud SQL
  - Injectable KubeExec fake for offline unit coverage
  - Exported Query helper for harness table assertions
affects: [07-06 live harness dump cell, MIGRATE-04 managed half]

tech-stack:
  added: []
  patterns:
    - "Options.Runner=kube + kubectl exec env MYSQL_PWD + injectable KubeExec"
    - "Job/sidecar fallback documented in kubeMySQL when image lacks mysql client"

key-files:
  created:
    - internal/dumpimport/runner_kube.go
    - internal/dumpimport/runner_kube_test.go
    - internal/dumpimport/options_defaults_test.go
  modified:
    - internal/dumpimport/import.go
    - internal/dumpimport/import_test.go
    - internal/dumpimport/doc.go

key-decisions:
  - "Prefer kubectl exec piping SQL over Cloud SQL Auth Proxy+IAP for managed dump connectivity"
  - "Password via MYSQL_PWD env arg to in-pod mysql — never -pPASSWORD (T-07-08)"
  - "Do not mark MIGRATE-04 Complete — live evidence is 07-07"

patterns-established:
  - "Pattern: dumpimport RunnerKube with KubeExec injection for zero-cluster unit tests"
  - "Pattern: Host defaults 127.0.0.1:3306 only when Runner unset; kube requires explicit private IP Host"

requirements-completed: []  # MIGRATE-04 deferred to 07-07 live evidence (plan instruction)

coverage:
  - id: D1
    description: "Kube-adjacent dumpimport runner imports tiny.sql via fake kubectl exec"
    requirement: MIGRATE-04
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1 -run Kube"
        status: pass
    human_judgment: false
  - id: D2
    description: "Exec failures surface loudly; host defaults remain loopback when kube unset"
    requirement: MIGRATE-04
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-30
status: complete
---

# Phase 07 Plan 04: Kube-adjacent dumpimport runner Summary

**Private-IP Cloud SQL dump path via kubectl-exec runner with offline fake-exec unit coverage (zero cloud spend).**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-30T11:40:55Z
- **Completed:** 2026-07-30T11:44:30Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- Added `RunnerKube` transport: `kubectl exec -i` into Magento/web (or Job) pod piping SQL to in-pod `mysql` against Cloud SQL private IP
- Injectable `KubeExec` enables tiny.sql import + table/probe verification without a cluster
- Kept Phase 5 nonempty/--yes safety; host/compose defaults unchanged when Runner unset
- Exported `Query` for harness cells to assert fixture tables after managed dump

## Task Commits

1. **Task 1: End-to-end kube-adjacent dumpimport runner** - `703798c` (feat)
2. **Task 2: Assert tables + failed-loud runner errors** - `7126309` (test)

**Plan metadata:** (pending docs commit)

## Files Created/Modified

- `internal/dumpimport/runner_kube.go` — kubeMySQL runner + DefaultKubeExec + Job/sidecar fallback comment
- `internal/dumpimport/runner_kube_test.go` — fake exec + table-driven success/failure/nonempty
- `internal/dumpimport/options_defaults_test.go` — host loopback defaults; kube does not invent 127.0.0.1
- `internal/dumpimport/import.go` — Options kube fields, resolveRunner branch, Query export
- `internal/dumpimport/import_test.go` — host-mode defaults smoke when kube unset
- `internal/dumpimport/doc.go` — document RunnerKube transport

## Decisions Made

- kubectl exec over Auth Proxy+IAP for the managed dump connectivity path (plan discretion)
- MYSQL_PWD via `env` in exec argv for in-pod mysql (T-07-08); never log connection strings
- MIGRATE-04 left Incomplete in REQUIREMENTS until 07-07 live evidence

## Deviations from Plan

None - plan executed exactly as written.

## Threat Flags

None — surface matches plan threat model (T-07-07 nonempty/--yes retained; T-07-08 MYSQL_PWD; no new packages).

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: internal/dumpimport/runner_kube.go
- FOUND: internal/dumpimport/runner_kube_test.go
- FOUND: 703798c
- FOUND: 7126309
- Verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1` → 14 passed
