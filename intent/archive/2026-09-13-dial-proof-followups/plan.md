---
status: planned
slug: dial-proof-followups
spec: spec.md
---

# Plan: close the Dial proof test gaps

Auto-approved per the standing `/goal` instruction.

## Files that change

- NEW `internal/providerhost/plan_equality_test.go`: Dial-vs-local
  Plan equality plus the GCP fixture.
- NEW `internal/providerhost/dial_mismatch_test.go`: TestMain helper
  plus the mismatch test.

## Order of work

- [x] 1.1 Plan equality test — verify: passes; fails if the opaque
  spec is mutated (sanity-check by temporary mutation, then revert)
- [x] 1.2 Dial mismatch test — verify: passes; helper path serves
  only under the env var
- [x] 1.3 Providerhost suite plus lint — verify: package green,
  linter clean

## Risks

- Fixture drift if `PlanFromConfig` gains required fields: the test
  fails loudly, which is the point.

## Proof

Package test log, lint log.
