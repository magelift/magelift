---
schema_version: 1
open_count: 2
waived_count: 0
fixed_count: 0
total_count: 2
last_updated: 2026-07-28T11:28:53.828Z
---

# Broken Windows Ledger

> Cross-phase defect register. `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 01 | unrun-verify | docs/lint-policy.md |  | deferred-ci: CI go test -race ./... measurement and force-all eight-target verify run | open |  | 2026-07-28T11:28:53.760Z |  |
| 2 | 01 | unrun-verify | .github/workflows/ci.yml |  | deferred-local: full GOMAXPROCS=1 go test -race ./... not run under Cursor | open |  | 2026-07-28T11:28:53.828Z |  |

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
  }
]
````
