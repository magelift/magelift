# ADR 0012: Search proof shapes and spend cap

- Status: Accepted
- Date: 2026-09-13

## Context

Certified search is `disabled`, and a catalog shop without search is a
spike. V1 proves one live Magento search cell per certified origin. The
AWS proof must spend the minimum necessary credits inside the packed
AWS session; the GCP proof runs on the unlimited project. This ADR
records the shapes, the cap, and the bar. The live runs belong to the
Phase 2 intent `magento-search-live-proof`.

## Decision

### AWS provisioned cell

- Engine `OpenSearch_3.1` (repo default; Magento 2.4.8/2.4.9 compat
  policy requires the `OpenSearch_3` prefix).
- One `t3.small.search` data node, single AZ, no dedicated masters.
- 10 GB gp3 EBS, baseline throughput.
- HTTPS on 443 in-VPC, no SigV4 sidecar. Fine-grained access control
  with an internal user; the password lives in Secrets Manager and the
  task role reads that secret only.
- YAML fields: `searchMode: provisioned`, `instanceType:
  t3.small.search`, `instanceCount: 1`, `ebsVolumeType: gp3`,
  `ebsVolumeSizeGiB: 10`.
- Domain lifetime 8 hours maximum, destroyed immediately after proof,
  attached to the packed AWS session. No standalone search stack.

### GCP workload cell

- One-replica OpenSearch StatefulSet on Autopilot, pinned
  `opensearchproject/opensearch:3` image, 10 GB persistent disk,
  single-node discovery.
- Data must survive a pod delete: after recycle, Magento reconnects
  and catalog queries work with no manual reindex, or the cell does
  not certify.
- Destroyed on session exit with the spend recorded.

### Spend cap

- AWS search cell cap: $5. Expected spend at list prices is under
  $0.50 for a four-hour session ($0.036/hr per t3.small in us-east-1
  per AWS list prices mirrored July 2026; EU regions slightly higher;
  10 GB gp3 adds cents). The cap holds 10x headroom for region uplift
  and a slow session.
- One attempt on AWS. A single failure keeps the GCP proof, leaves AWS
  search experimental with the failure recorded, and re-plans instead
  of retrying into the credits.

### Proof criteria

Each cell demonstrates, in order: Magento reindex exit 0 on the
proof catalog, a storefront search query returning catalog hits, and
reconnect after recycle (pod delete on GCP, task recycle on AWS) with
queries working and no manual reindex. AWS adds a least-privilege
review: task role, security group reachability, and secret read scope.

### Evidence checklist

The packed sessions produce: reindex output, query request plus
response, recycle plus re-query output, destroy plus clean assertion,
and the spend line. Certification claims wait for these artifacts;
no matrix or release-readiness edits land with this decision.

## Consequences

- Phase 2 runs the two cells without re-litigating shape or budget.
- AOSS serverless stays experimental: its OCU floor costs more than
  the provisioned shape for a short proof run.
- Three-node HA stays out: it needs GKE Standard sysctl, experimental.

## Alternatives considered

- AOSS serverless for AWS: rejected (minimum OCUs bill from the first
  minute; provisioned t3.small is an order of magnitude cheaper short
  term).
- Larger provisioned shape (m6g.large, multi-AZ): rejected (HA is not
  the claim; the proof catalog fits t3.small comfortably).
- 3-replica workload on Autopilot: rejected (multi-node discovery is
  the HA claim and belongs on GKE Standard, experimental).

## Provenance

`internal/cloud/aws/search` (provisioned fields),
`internal/cloud/gcp/search` (workload shape),
`internal/config` compat policy plus defaults. AWS OpenSearch and EBS
list prices via public mirrors, checked September 2026; Phase 2
re-checks before the run.
