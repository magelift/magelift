---
status: draft
slug: aws-autonomous-provider
---

# Intent: AWS autonomous provider

## Problem

AWS ECS is already implemented and has valuable live evidence, but the shipped
core still registers AWS in process while GCP runs as an autonomous provider.
If AWS feature work resumes in that form, shared application and deployment
logic will continue to have two integration shapes and the provider boundary
will remain GCP-specific.

AWS is the required second provider immediately after the GCP alpha. Rewriting
the artifact, deployment lifecycle, trust path, or operations framework for AWS
would waste the work used to reach GCP.

## Evidence

[The architecture report](../report.md) identifies the contracts AWS can reuse
and the topology it must continue to own.

[The provider plugin ADR](../../docs/adr/0013-provider-plugin-contract.md)
requires provider-owned Pulumi execution and typed operations, and records that
full provider independence is due no later than AWS parity.

[The capability matrix](../../docs/capability-matrix.md) records bounded AWS
ECS Fargate evidence. The clean-room field note
[AWS Magento wiring follows a prior shop](../../.agents/knowledge/lessons/AWS%20Magento%20wiring%20follows%20a%20prior%20shop.md)
is design input only, not certification.

## Proposed outcome

AWS ECS ships as a separately built, signed, automatically acquired provider
that consumes the same application artifact, deploy phases, operation
protocol, progress/errors, release journal, and distribution trust path as
GCP.

AWS owns its network, IAM, ECS, database, cache, search, queue, storage, edge,
observability, cost, recovery, and cleanup behavior. The existing ECS
acceptance scenarios run through the autonomous provider. Differences from GCP
are explicit instead of normalized away.

The work proves that adding AWS requires provider implementation and evidence,
not a second Magento platform.

## Affected users and systems

AWS pilot teams; root CLI dependency closure; SDK and provider protocol;
internal AWS packages and new providers/aws module; release catalog; AWS
onboarding, capability matrix, and evidence.

## Constraints

- Starts only after the GCP alpha decision.
- Remove provider imports of root-internal helpers before copying the GCP
  boundary into AWS.
- The shipped core imports no AWS or GCP cloud/Pulumi provider SDK.
- AWS uses the same signed lock, automatic acquisition, negotiation, and
  fail-closed policy as GCP.
- Provider topology remains AWS-owned; no shared Pulumi graph with provider
  switches.
- Shared contract changes keep GCP green.
- Live AWS work requires a refreshed budget, smallest useful shape, destroy,
  assert-clean, and evidence.
- Existing AWS certification claims are neither inherited nor discarded; they
  are mapped to the exact autonomous-provider path they prove.

## Out of scope

- New AWS feature breadth not needed for existing ECS parity.
- EKS certification.
- Aurora, Amazon MQ, or paid search expansion without a separately justified
  user need and budget.
- GCP topology changes.
- EU providers.
- Stable provider API or independent provider cadence.

## Open questions

- Which existing AWS capabilities must be in the first autonomous-provider
  release versus honestly retained as experimental? Decide from current
  evidence and pilot needs in the spec; do not mirror GCP topology mechanically.
