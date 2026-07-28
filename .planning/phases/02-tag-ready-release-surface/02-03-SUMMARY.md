---
phase: 02-tag-ready-release-surface
plan: 03
subsystem: extensibility
tags: [custom-cli, community-provider, register-module, release-03]

requires:
  - phase: 02-tag-ready-release-surface
    provides: 02-01 RC contract surfaces
provides:
  - Clean-GOMODCACHE verification recipe in adding-a-provider + custom-cli README
  - stubModule community registration slot with Plan refusal
  - Proven empty-cache build + version: dev
affects: [02-04-packaging-smoke]

tech-stack:
  added: []
  patterns: [compile-time custom binary community registration; internal/ honesty]

key-files:
  created:
    - examples/custom-cli/stub_module.go
    - examples/custom-cli/stub_module_test.go
  modified:
    - docs/adding-a-provider.md
    - examples/custom-cli/README.md
    - examples/custom-cli/main.go

key-decisions:
  - "Community providers are compile-time custom binaries of this module; separate modules cannot import internal/"
  - "stubModule uses example/community-stub IDs + TierExperimental; Plan returns ErrNotSupported-wrapped refusal"

patterns-established:
  - "Clean empty GOMODCACHE+GOCACHE go build ./examples/custom-cli is the RELEASE-03 proof"

requirements-completed: [RELEASE-03]

coverage:
  - id: D1
    description: Docs include clean-GOMODCACHE recipe and internal/ honesty
    requirement: RELEASE-03
    verification:
      - kind: other
        ref: "rg GOMODCACHE docs/adding-a-provider.md examples/custom-cli/README.md"
        status: pass
    human_judgment: false
  - id: D2
    description: Clean empty GOMODCACHE build of custom-cli + version
    requirement: RELEASE-03
    verification:
      - kind: other
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go build -o /tmp/magelift-ext-phase2 ./examples/custom-cli → version: dev"
        status: pass
    human_judgment: false
  - id: D3
    description: stubModule registered; Plan refuses; clean-cache build after stub
    requirement: RELEASE-03
    verification:
      - kind: unit
        ref: "go test ./examples/custom-cli/"
        status: pass
      - kind: other
        ref: "go build -o /tmp/magelift-ext-stub ./examples/custom-cli → version: dev"
        status: pass
    human_judgment: false

duration: 22min
completed: 2026-07-28
status: complete
---

# Phase 2 Plan 03: Custom-CLI Community Registration Summary

**Clean-GOMODCACHE build of `examples/custom-cli` succeeds with `version: dev`, docs state `internal/` honesty, and a literal `stubModule` registration slot refuses deploy.**

## Performance

- **Duration:** ~22 min (dominated by two empty-cache serial `go build` downloads)
- **Started:** 2026-07-28T14:46:00Z
- **Completed:** 2026-07-28T15:06:48Z
- **Tasks:** 2/2
- **Files modified:** 5

## Accomplishments

- `docs/adding-a-provider.md` + `examples/custom-cli/README.md` carry matching clean-cache recipes and compile-time / `internal/` honesty
- Empty `GOMODCACHE`+`GOCACHE` build: exit 0, `version: dev` (pre-stub and post-stub)
- `stubModule` (`example.community-stub`) registered from `main`; `Plan`/`Program` return `ErrNotSupported`-wrapped refusals
- TDD: RED compile-fail → GREEN `go test ./examples/custom-cli/` pass

## Task Commits

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Clean-GOMODCACHE recipe (tracer) | ca0db6b |
| 2a | RED failing stub tests | 007d4ed |
| 2b | GREEN stub + RegisterModule wire | ae8f08f |

## Evidence

```text
# tracer clean-cache
version: dev
BUILD_EXIT=0

# post-stub clean-cache
version: dev
go test ./examples/custom-cli/ → ok
```

## Deviations from Plan

None - plan executed exactly as written.

## TDD Gate Compliance

- RED: `007d4ed` (`test(02-03): ...`)
- GREEN: feat commit after RED
- No refactor commit needed

## Known Stubs

| File | Line | Reason |
| --- | --- | --- |
| examples/custom-cli/stub_module.go | Plan/Program | Intentional demo — refuses deploy; real providers replace these |

## Self-Check: PASSED

- FOUND: docs/adding-a-provider.md, examples/custom-cli/{README.md,main.go,stub_module.go,stub_module_test.go}
- FOUND: ca0db6b, 007d4ed, ae8f08f
- Clean-cache builds exited 0 with `version: dev`
