---
status: done
slug: magento-search-v1
intent: intent.md
---

# Spec: search proof shapes and spend cap (decision half)

Auto-approved per the standing `/goal` instruction. This spec covers the
decision only; the live runs belong to `magento-search-live-proof`.

## Requirements

### Requirement: AWS provisioned proof shape

The decision SHALL name one AWS provisioned shape: engine, instance
type, node count, EBS type and size, AZ layout, and auth path, chosen
as the cheapest shape that still proves Magento index, query, and
reconnect credibly.

#### Scenario: shape is runnable from config

- **WHEN** a Phase 2 operator reads the ADR
- **THEN** every provisioned field maps to a `magelift.yaml` search
  field (`instanceType`, `instanceCount`, `ebsVolumeType`,
  `ebsVolumeSizeGiB`) plus the engine default, with no missing value

### Requirement: GCP workload proof shape

The decision SHALL name the GCP OpenSearch workload shape: replica
count, storage, discovery mode, and whether data must survive a pod
recycle.

#### Scenario: recycle bar is explicit

- **WHEN** a Phase 2 operator reads the ADR
- **THEN** the recycle test states pass or fail plainly: Magento
  reconnects after a pod delete, and catalog data survives without a
  manual reindex, or the cell does not certify

### Requirement: written AWS spend cap

The decision SHALL state a dollar cap for the AWS search cell, a
maximum domain lifetime, the expected spend at list prices, and the
one-attempt rule on failure.

#### Scenario: cap bounds the session

- **WHEN** the packed AWS session runs the search cell
- **THEN** the cap plus the lifetime rule plus immediate destroy bound
  the spend before the run starts, and a single failure keeps the GCP
  proof while AWS search stays experimental with the failure recorded

### Requirement: proof criteria and evidence

The decision SHALL list what each cell must demonstrate (index, query,
reconnect, least-privilege review on AWS) and which evidence artifacts
the packed sessions must produce.

#### Scenario: evidence checklist exists

- **WHEN** Phase 2 closes a search cell
- **THEN** the ADR checklist names each artifact: reindex output, query
  response, recycle plus re-query, destroy plus clean assertion, and
  the spend line

## Design

One ADR (0012) holds the whole decision: shapes, cap, criteria,
evidence, and the link to the Phase 2 implementation intent. No code
changes; no matrix or release-readiness edits (certification claims
wait for evidence). `ROADMAP.md` order 9 points at
`magento-search-live-proof`.

## Gotchas / policy flags

- Engine stays OpenSearch 3 (`OpenSearch_3.1` default, pinned `:3`
  workload image): the repo compat policy already requires it for
  Magento 2.4.8/2.4.9.
- AOSS serverless stays experimental; the provisioned path avoids its
  OCU floor for a short proof run.
- Three-node HA stays out: it needs GKE Standard sysctl, experimental.
