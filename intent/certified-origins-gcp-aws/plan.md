---
status: planned
slug: certified-origins-gcp-aws
spec: spec.md
---

# Plan: two certified origins proved on a tight testing budget

Auto-approved per the standing `/goal` instruction.

## Files that change

- Evidence: NEW `docs/evidence/*` session records (GCP plus AWS).
- EDIT `docs/capability-matrix.md`: only if a cell status changes.
- EDIT `docs/evidence/README.md`: index the new records.
- Code: only if a live run exposes a defect (fix plus test, logged
  as Deviation).

## Order of work

- [ ] 1.1 Pyramid base — verify: `make local-gates` exit 0
- [x] 1.2 Maintainer unblock: gcloud re-auth plus project confirm
  plus ephemeral-tag permission — verify: `gcloud` identity works
  and the project is named in writing
- [ ] 1.3 Ephemeral CI bundle (`v0.0.0-dialproof.N`) — verify:
  workflow green, bundle plus binary downloaded, tag deleted after
  use
- [ ] 1.4 GCP packed session: preview, standard, HA, live Dial
  proof, order-9 GCP search cell, order-10 preview loop —
  verify: evidence plus destroy plus assert_clean plus spend line
- [ ] 1.5 AWS packed session: preview create-once plus warm
  transitions, order-9 AWS search cell — verify: evidence plus
  destroy plus assert_clean, spend inside $25
- [ ] 1.6 Matrix plus evidence index update — verify: docs build

## Risks

- GCP auth/project unconfirmed: session cannot start without the
  maintainer (box 1.2 is a question, not work).
- AWS retry margin: one retry inside the cap, then AWS goes
  experimental honestly.
- Long sessions must not be interrupted mid-create/destroy.

## Proof

Gate log, session transcripts, evidence files, spend lines,
assert_clean outputs.
