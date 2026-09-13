---
status: planned
slug: v1-stable-cut
spec: spec.md
half: engineering
---

# Plan: v1-stable-cut engineering half

Auto-approved per the standing `/goal` instruction.

## Files that change

- EDIT `internal/upgrade/upgrade.go`: Windows zip asset plus zip
  extraction.
- EDIT `internal/upgrade/upgrade_test.go`: zip plus tar.gz Install
  fixtures.
- EDIT `internal/cli/subprocess.go`: lock-vs-CLI skew warning.
- EDIT `internal/cli/subprocess_test.go`: skew warning test.
- EDIT `scripts/release-smoke-local.sh`: provider binary assertion.
- EDIT `.github/workflows/release.yml`: macOS cask-verify job.
- EDIT `docs/release-readiness.md`: fresh smoke plus license records.
- EDIT `docs/install.md`: only if the asset-name audit finds drift.
- EDIT `docs/post-beta-roadmap.md`: provider co-upgrade row.

## Order of work

- [x] 1.1 Windows upgrade plus fixtures — verify: upgrade package
  tests pin zip and tar.gz paths
- [x] 1.2 Skew warning plus test — verify: CLI test pins the warning
  text and versions
- [x] 1.3 Smoke script plus `make release-smoke` — verify: exit 0
  with both binaries; record the date
- [x] 1.4 Cask-verify job plus `make workflow-check` — verify: green
- [x] 1.5 `make license-check`, doc records, co-upgrade row,
  install audit — verify: docs build green
- [x] 1.6 Full suite plus lint on touched packages — verify: green

## Risks

- Release smoke is slow on this runner (single-target serial);
  allow a long block window.

## Proof

Package tests, smoke log, workflow-check log, license log, suite.
