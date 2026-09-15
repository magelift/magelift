---
status: planned
slug: operator-finops-catalog
spec: spec.md
---

# Plan: one command catalog for run, debug, and spend

Auto-approved per the standing `/goal` instruction.

## Files that change

- EDIT the five stack packages (`aws`, `aws/eksops`, `gcp`, `ovh`,
  `scaleway`): carry allow-expired into the Pulumi program, skip only
  the expiry check on destroy.
- EDIT stack tests per provider: expired destroys, expired deploys
  refuse.
- EDIT `docs/operations.md`: five-verb proof paragraph.
- Session only: GCP packed session plus AWS creds-only cost reads
  (no code unless a defect surfaces).

## Order of work

- [x] 2.1 Destroy-after-expiry fix, all providers — verify: new unit
  tests green per provider, existing suites green (232 passed, 5
  packages; plus the isOnlyExpirationError over-forgiveness fix)
- [x] 2.2 GCP five-verb packed session — verify: five rows green,
  interrupted run reconciled, destroy plus assert clean, spend line
  (mldp8: 13/13 cells, 5 verbs green, expired-YAML destroy
  succeeded after deploy refused, zero leftovers except the
  tombstoned WIF pool name)
- [x] 2.3 AWS creds-only cost reads — verify: live prices plus
  unpriced list, budget read without forecast (Fargate $17.74 +
  $3.87, Valkey $10.51; 5 filter defects fixed; budget page-size
  fixed; preview not-configured plus staging configured proven)
- [x] 2.4 Floci plus unit for the rest — verify: evidence/audit no
  secrets, tunnel paths, budget presentation tests green (323 unit
  + Floci AWS/GCP suites green)
- [x] 2.5 Docs plus skill sync — verify: docs build green
  (operations.md verbs section, skill commands+rules, manifest
  regenerated, evidence + README, docs build green)

## Risks

- The in-program fix must not weaken any check except expiry;
  deploy-path strictness is pinned by the same tests.
- GCP session cost is negligible but the project must still end
  empty (packed-session discipline).

## Proof

Unit tests, session transcript, evidence rows, spend lines.
