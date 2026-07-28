---
schema_version: 1
open_count: 8
waived_count: 0
fixed_count: 0
total_count: 8
last_updated: 2026-07-28T14:45:35.231Z
---

# Broken Windows Ledger

> Cross-phase defect register. `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 01 | unrun-verify | docs/lint-policy.md |  | deferred-ci: CI go test -race ./... measurement and force-all eight-target verify run | open |  | 2026-07-28T11:28:53.760Z |  |
| 2 | 01 | unrun-verify | .github/workflows/ci.yml |  | deferred-local: full GOMAXPROCS=1 go test -race ./... not run under Cursor | open |  | 2026-07-28T11:28:53.828Z |  |
| 3 | 01 | unrun-verify | internal/platform/cost_test.go |  | deferred-local: go test -race ./internal/platform/ aborted — ops_test AWS race compile exhausted free RAM; verified without -race | open |  | 2026-07-28T11:37:00.000Z |  |
| 4 | 01 | deviation | internal/cloud/aws/runtime/runtime.go | 740 | F-01-07-1: nginx-fpm queue/deploy/cron get search-proxy container but no DependsOn (appendSearchProxy only wires php-fpm) | open |  | 2026-07-28T13:42:02.803Z |  |
| 5 | 01 | unrun-verify | internal/cloud/aws/stack/component_test.go |  | go test -race deferred-local under Cursor after swap exhaustion; non-race package verify passed | open |  | 2026-07-28T13:42:02.890Z |  |
| 6 | 01 | deviation | internal/cloud/aws/stack/component_test.go |  | preview×amazon-mq rejection now exercises queue AZ guard (c7435d2), not Spec.Validate zone-count | open |  | 2026-07-28T13:46:30.826Z |  |
| 7 | 01 | todo | internal/cloud/aws/runtime/runtime.go |  | F-01-07-1: search-proxy DependsOn skipped for nginx-fpm queue/deploy/cron Magento containers | open |  | 2026-07-28T13:46:30.886Z |  |
| 8 | 02 | unmet-truth | CONTRIBUTING.md |  | RELEASE-06 fresh-clone make verify blocked: PHP 8.2+ and Composer missing on proof host | open |  | 2026-07-28T14:45:35.231Z |  |

````json
[
  {
    "id": 1,
    "kind": "unrun-verify",
    "phase": "01",
    "file": "docs/lint-policy.md",
    "line": null,
    "description": "deferred-ci: CI go test -race ./... measurement and force-all eight-target verify run",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T11:28:53.760Z",
    "resolved_at": null
  },
  {
    "id": 2,
    "kind": "unrun-verify",
    "phase": "01",
    "file": ".github/workflows/ci.yml",
    "line": null,
    "description": "deferred-local: full GOMAXPROCS=1 go test -race ./... not run under Cursor",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T11:28:53.828Z",
    "resolved_at": null
  },
  {
    "id": 3,
    "kind": "unrun-verify",
    "phase": "01",
    "file": "internal/platform/cost_test.go",
    "line": null,
    "description": "deferred-local: go test -race ./internal/platform/ aborted — ops_test AWS race compile exhausted free RAM; verified without -race",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T11:37:00.000Z",
    "resolved_at": null
  },
  {
    "id": 4,
    "kind": "deviation",
    "phase": "01",
    "file": "internal/cloud/aws/runtime/runtime.go",
    "line": 740,
    "description": "F-01-07-1: nginx-fpm queue/deploy/cron get search-proxy container but no DependsOn (appendSearchProxy only wires php-fpm)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T13:42:02.803Z",
    "resolved_at": null
  },
  {
    "id": 5,
    "kind": "unrun-verify",
    "phase": "01",
    "file": "internal/cloud/aws/stack/component_test.go",
    "line": null,
    "description": "go test -race deferred-local under Cursor after swap exhaustion; non-race package verify passed",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T13:42:02.890Z",
    "resolved_at": null
  },
  {
    "id": 6,
    "kind": "deviation",
    "phase": "01",
    "file": "internal/cloud/aws/stack/component_test.go",
    "line": null,
    "description": "preview×amazon-mq rejection now exercises queue AZ guard (c7435d2), not Spec.Validate zone-count",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T13:46:30.826Z",
    "resolved_at": null
  },
  {
    "id": 7,
    "kind": "todo",
    "phase": "01",
    "file": "internal/cloud/aws/runtime/runtime.go",
    "line": null,
    "description": "F-01-07-1: search-proxy DependsOn skipped for nginx-fpm queue/deploy/cron Magento containers",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T13:46:30.886Z",
    "resolved_at": null
  },
  {
    "id": 8,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "CONTRIBUTING.md",
    "line": null,
    "description": "RELEASE-06 fresh-clone make verify blocked: PHP 8.2+ and Composer missing on proof host",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-07-28T14:45:35.231Z",
    "resolved_at": null
  }
]
````
