---
status: superseded
slug: eu-providers-experimental
spec: spec.md
---

> HISTORICAL 2026-09-16: retained as-is (Scaleway GREEN, OVH PARKED).
> Superseded as an active plan by `intent/audit.md` — EU work is deferred
> past alpha. Resume here.

# Plan: honest EU provider path without draining the card

Auto-approved per the standing `/goal` instruction.

## Files that change

- REPLACE `docs/evidence/scaleway-kapsule-infrastructure-live-*.md`
  and `docs/evidence/ovh-mks-infrastructure-live-*.md` with September
  runs (replace, not append).
- EDIT `docs/evidence/README.md` only if row text changes.
- Session only: live runs plus spend lines (no code unless a defect
  surfaces).

## Order of work

- [x] 1.1 Offline re-verify — verify: `go test`
  `./internal/cloud/ovh/... ./internal/cloud/scaleway/...` green,
  both dry-run harnesses green
  (284 passed in 24 packages; both dry-runs ok)
- [x] 1.2 Scaleway live refresh — verify: harness `assert_clean ok`
  plus direct inventories empty plus spend line
  (`scw915` nl-ams-1 GREEN exit 0, provision 13m12s, teardown 3m0s,
  direct sweep zero; password-policy fix plus catalog defaults;
  <€1 inside the $50 cap)
- [ ] 1.3 OVH live refresh (after 1.2, serialized) — verify: same
  three
  (PARKED 2026-09-15: 6 attempts, 0 PASS — MKS pools do not converge
  (PAR b3-8 INSTALLING-stuck, B2 flavors 404, MIL+b3-8 inconclusive);
  August evidence stands; retry another day per user call)
- [ ] 1.4 Evidence plus matrix check — verify: docs build green, EU
  rows still experimental

## Risks

- August catalog versions (Kapsule, Redis, MySQL, MKS plan) may have
  aged out; the harness admission step names the current catalog
  before mutation.
- The OVH subnet 409 retry path may need its patience again.

## Proof

Harness transcripts, replaced evidence files, spend lines.
