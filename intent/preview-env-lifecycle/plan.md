---
status: planned
slug: preview-env-lifecycle
spec: spec.md
---

# Plan: long-running envs plus cheap short-lived previews

Auto-approved per the standing `/goal` instruction.

## Files that change

- EDIT `docs/operations.md`: preview promise paragraph.
- EDIT config tests: pin preview resolution per origin.
- Session only: loop transcript plus evidence rows (no code
  unless a defect surfaces).

## Order of work

- [x] 1.1 Preview-default pins plus docs — verify: new config
  tests green, docs build green
- [ ] 1.2 Live CLI loop inside the GCP packed session —
  verify: transcript plus evidence rows plus assert_clean
- [x] 1.3 Guard suite unchanged — verify: stale-event plus
  production-gate tests green

## Risks

- NAT option names differ per origin; the pin must use each
  origin's own vocabulary.

## Proof

Unit tests, session transcript, evidence rows.
