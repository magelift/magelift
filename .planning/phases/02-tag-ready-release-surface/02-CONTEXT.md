---
phase: 02-tag-ready-release-surface
generated: 2026-07-28T14:32:31Z
source: auto-from-roadmap (yolo; discuss skipped — user "start phase 2")
---

# Phase 2 Context: Tag-Ready Release Surface

## Locked Decisions

1. First public tag is `v1.0.0-rc.1` — named in README, docs/versioning.md, docs/release-readiness.md; strip remaining pre-alpha / v0.x product claims (CHANGELOG history excepted).
2. `docs/versioning.md` must include an RC stability statement for `sdk/v1` and `platform.StackModule`, explicitly reserving shared-Kubernetes port changes for Phase 6.
3. Packaging gate closes via `make release-smoke` / `./scripts/release-smoke-local.sh` — serial, single-target only (AGENTS.md). Record run date + output on the release-readiness board. Prefer plain Terminal for the heavy smoke; agents may prepare the script/docs but must not parallelize goreleaser.
4. CONTRIBUTING.md alone must take a fresh clone to green `make verify` (or honest substitute when hosted CI minutes are still exhausted — document the Phase 1 HUMAN_GATE: Actions minutes deferred; local verify path must be accurate).
5. `examples/custom-cli` + `docs/adding-a-provider.md` alone must register an out-of-tree provider from a clean module cache (no consulting core source during the verification).
6. RELEASE-05 (full gate-board Close/Defer on tag day) stays Phase 8 — out of Phase 2 scope.
7. Phase 1 hosted CI green remains HUMAN_GATE / deferred; Phase 2 proceeds under that accepted deferral per maintainer direction 2026-07-28.

## Agent Discretion

- How to split plans across docs vs packaging vs contributor-path vs custom-cli verification
- Exact wording of stability statement as long as Phase 6 reservation is explicit
- Whether packaging-smoke evidence is committed as a snippet file vs board table row only

## Deferred

- Hosted force-all CI proof (Phase 1 QUALITY-06) until Actions minutes return
- RELEASE-05 tag-day board audit → Phase 8
- F-01-07-1 search-proxy DependsOn → later correctness / Phase 6 adjacency

## Scope Fence

IN: version story docs, RC contract statement, release-smoke packaging gate closure evidence, CONTRIBUTING verify path, custom-cli out-of-tree provider proof.
OUT: paid cloud, acceptance harness (Phase 3), import/migrate, kube day-2, GCP cert, attach, RELEASE-05.
