---
status: specified
slug: magento-search-live-proof
intent: intent.md
---

# Spec: live Magento search on both certified origins

Auto-approved per the standing `/goal` instruction. Shapes, caps,
and criteria are inherited frozen from ADR 0012; this spec only
binds them to sessions.

## Requirements

### Requirement: GCP workload cell proves search

Inside the GCP packed session, the preview stack SHALL run the
OpenSearch workload cell: Magento reindex exit 0 on the proof
catalog, a storefront search query returning catalog hits, pod
delete, then queries working with no manual reindex.

#### Scenario: workload survives recycle

- **WHEN** the pod is deleted after a green reindex plus query
- **THEN** Magento reconnects and catalog queries work with no
  manual reindex

### Requirement: AWS provisioned cell proves search once

Inside the AWS packed session, the preview stack SHALL warm-patch
to the ADR 0012 provisioned shape and demonstrate reindex exit 0,
query hits, reconnect after task recycle, and a least-privilege
review (task role, SG reachability, secret scope). One attempt;
lifetime 8h max; destroy immediately after.

#### Scenario: provisioned search green or honestly experimental

- **WHEN** the single attempt completes
- **THEN** either all four proofs hold with evidence, or AWS search
  is recorded experimental with the failure

### Requirement: no standalone search stacks

Both cells SHALL run on their origin session's stack and die with
it.

#### Scenario: attached only

- **WHEN** Phase 2 closes
- **THEN** no search domain or workload outlived its session

## Design

GCP: the preview catalog already creates and checks OpenSearch; the
proof sequence (reindex, query, pod delete, re-query) runs against
the live preview stack via `magelift exec` / Magento CLI, recorded
as evidence rows. AWS: warm-patch `searchMode: provisioned` with
the ADR 0012 fields on the packed preview stack, prove, destroy.

## Gotchas / policy flags

- Never retry AWS into the credits: one attempt, then re-plan.
- The 8h AWS domain cap is a ceiling; destroy immediately after
  proof regardless.

## Open questions carried forward

None.
