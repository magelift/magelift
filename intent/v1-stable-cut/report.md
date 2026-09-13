---
status: engineering-verified
slug: v1-stable-cut
plan: plan.md
verdict: engineering-pass
---

# Report: v1-stable-cut engineering half (order 7)

The tag, README flip, and install-doc flip stay order 14. This report
covers the release wiring only. The intent stays open until the tag
cycle archives it.

## What shipped

- Windows upgrade fixed: `upgrade` requests the `.zip` asset on
  Windows (matching the GoReleaser `format_overrides`) and extracts
  from zip with the same guards as tar (day-0 bug: it requested a
  nonexistent `.tar.gz` and only read tar).
- Provider skew warning: loading a provider whose lock version
  differs from the CLI version warns on stderr naming both; dev
  builds and versionless locks stay quiet. Full co-upgrade recorded
  as post-v1.
- Smoke covers the provider binary: `make release-smoke` asserts
  both `magelift` and `magelift-provider-gcp` in `dist/`.
- `cask-verify` job in `release.yml`: after release, a macOS runner
  taps the published cask, installs it, and runs `magelift version`.
  This is the tested macOS install path the readiness doc requires.
- Records current: smoke plus license records dated 2026-09-13,
  gate-board rows updated, install doc audited against the actual
  asset names (no drift, no change).

## Deviations

None. Two decisions taken and recorded in the spec: cask publishes
at the tag with in-pipeline verification (replaces "cask optional
post-tag"); upgrade stays CLI-only for v1 with a skew warning.

## Evidence

- `go test ./... -count=1`: exit 0, 133 packages ok
  (`/tmp/test-v1cut.log`).
- Upgrade package: 6 passed (tar.gz plus zip Install paths).
- `make release-smoke`: exit 0, both binaries reported
  (`/tmp/release-smoke-v1cut.log`).
- `make workflow-check`: exit 0 (`/tmp/workflow-check-v1cut.log`).
- `make license-check`: exit 0 (`/tmp/license-v1cut.log`).
- Linter on `internal/upgrade` plus `internal/cli`: 0 issues
  (`/tmp/lint-v1cut.log`); gofmt clean; `make docs`: exit 0
  (`/tmp/docs-v1cut.log`).
- `gh secret list` shows `HOMEBREW_TAP_GITHUB_TOKEN` present.

## Notes for the tag cycle (order 14)

- `release.yml` line 39 cites an observation "(seen on v1.0.0-rc.1)"
  but no such tag exists; confirm the history or reword at tag time.
- The `cask-verify` job cannot run until a tag exists; its first
  green run is part of the tag proof.

## Verdict

Engineering pass. Phase 1 exit holds: local onboarding from docs,
classified deploy failures, Dial proof green locally, release
artifacts build and verify locally.
