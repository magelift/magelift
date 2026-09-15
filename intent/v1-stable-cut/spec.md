---
status: specified
slug: v1-stable-cut
intent: intent.md
half: tag (engineering half passed as order 7; see git history)
---

# Spec: v1-stable-cut tag half

Auto-approved per the standing `/goal` instruction. The order-7
engineering half is done (release wiring verified locally,
`verdict: engineering-pass` in history). This cycle covers roadmap
order 14 only: re-verify the tag gates on current HEAD and stop
before the tag. Tagging `v1.0.0-rc.1` is ask-first (release tag).

## Decisions

- The report ends with the exact tag command plus a go/no-go per
  gate. No tag is created by this order.
- Homebrew cask ships with rc.1: the gate board already closes the
  first-ship path that way, with the `cask-verify` pipeline job as
  the tested macOS install path. Its first green run is part of the
  tag proof. Windows stays archive download.
- The two `Pending` board rows (full architecture matrix, OVH/SCW
  certification) are certification scope, not tag blockers; the tag
  gates are the six rows at the top of `release-readiness.md`.
- Install docs flip to archives-first in the same change as the tag
  (they cannot point at archives before the tag exists). This order
  verifies the flip text is ready; it does not flip early.
- The order-7 note about phantom `v1.0.0-rc.1` citations in the
  workflows is resolved in this cycle (reword; no such tag exists).

## Acceptance criteria

- Fresh `make license-check` exit 0.
- Fresh `make release-smoke` exit 0 on current HEAD.
- CI for the release scope green (dispatch on the branch; failures
  triaged to mine, pre-existing, or out-of-scope with evidence).
- Release workflow inspection: GoReleaser archives plus checksums
  plus SBOM plus SLSA provenance plus keyless Sigstore plus
  `cask-verify` all present in the pipeline definition.
- `magelift upgrade` verify path covered by its tests.
- Gate board re-dated where this cycle re-verified a row.
