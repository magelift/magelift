---
status: planned
slug: v1-stable-cut
spec: spec.md
half: tag
---

# Plan: v1-stable-cut tag half

Auto-approved per the standing `/goal` instruction. Stops before the
tag (ask-first). Order-7 items stay complete in history.

## Files that change

- EDIT `docs/release-readiness.md`: re-date re-verified rows only.
- EDIT workflows only for the phantom-tag reword (done).
- Session only: license-check, release-smoke, CI dispatch, workflow
  inspection, upgrade-path tests.

## Order of work

- [x] 4.1 License check fresh — verify: `make license-check` exit 0
- [x] 4.2 Release smoke fresh — verify: `make release-smoke` exit 0
- [ ] 4.3 CI green for the release scope — verify: dispatched run
  triaged; my failures fixed, the rest evidenced
- [x] 4.4 Release workflow inspection — verify: archives, checksums,
  SBOM, SLSA, keyless Sigstore, cask-verify present
- [x] 4.5 Upgrade verify path — verify: tests green
- [ ] 4.6 Gate board re-date plus report — verify: report ends with
  the tag command plus go/no-go, no tag created

## Risks

- The release branch is `wip/all-local-work`, not `main`; the report
  must say so and the tag command must name the intended ref.
- Any red gate stops the order at 4.6 with no-go; fixing a gate is
  an explicit change, not a silent edit.

## Proof

Command outputs, workflow excerpts, CI run URLs, report with go/no-go.
