---
status: draft
slug: gcp-alpha-release
---

# Intent: first public GCP alpha

## Problem

MageLift has live evidence for important GKE Autopilot capabilities but no
public release whose documented installation path repeatedly builds, deploys,
serves, updates, and removes one real Magento shop from published artifacts.

The prior acceptance scope combined the first useful product proof with
generated CI and a broad day-two certification program. That prevented a
narrow alpha from shipping and made every application defect invalidate
unrelated distribution work.

## Evidence

[The architecture report](../report.md) separates distribution, artifact, GCP
product, project-pin, generated-CI, and extended-operation gates.

[The capability matrix](../../docs/capability-matrix.md) records GKE Autopilot,
Magento candidate deployment, one-replica OpenSearch, runtime operations, and
other bounded live evidence. It does not prove that the exact future public
artifacts complete the entire onboarding path.

The alpha development, provider delivery, and published image intents are
ordered prerequisites in [the roadmap](../ROADMAP.md).

## Proposed outcome

The pinned Magento Open Source GCP Autopilot recipe completes two clean
commit-addressed canaries:

- explicit account bootstrap;
- application build from released builder/runtime images;
- provider acquisition through the supported path;
- infrastructure apply;
- successful candidate deploy before traffic moves;
- public HTTPS Magento and static assets;
- known product and search behavior;
- cron and database queue behavior;
- media upload, fresh-pod retrieval, and relevant access-control negatives;
- an idempotent second deployment; and
- destroy with zero owned residual resources.

A release candidate built from the same commit and artifact identities passes
public distribution qualification and one bounded GCP confirmation. On green,
the report records a go/no-go and the exact ask-first alpha tag command. The
documented support claim names only the proved tuple.

## Affected users and systems

Pilot Magento teams; public installer and release assets; GCP provider;
reference shop; GCP onboarding and getting started; capability matrix and
evidence pack; release workflow.

## Constraints

- GCP GKE Autopilot remains the runtime.
- Use the smallest evidenced recipe defined by the roadmap.
- Use the same immutable artifact identities across canary and candidate
  qualification where their gate permits reuse.
- Live runs use the dedicated acceptance project, certification skill,
  ownership ledger, destroy on exit, residual inventory, and spend record.
- Public candidate qualification does not become the debugging loop.
- No tag is created without explicit user approval.
- Public docs and capability claims are updated together and processed through
  the required prose checks.
- Alpha language must not imply stable API or full production/HA coverage.

## Out of scope

- Generated GitHub CI.
- Full backup/restore, failed-release recovery, credential-expiry, autonomous
  preview expiry, HA, or disaster recovery unless required to protect an
  endpoint exposed by this alpha.
- GKE Standard and Cloud Run.
- AWS, EKS, OVH, Scaleway, edge vendors, and external observability.
- Stable v1.

## Open questions

- Which exact image digests and region become the published alpha tuple?
  Resolve from the two green canaries during the spec/plan; do not pin mutable
  tags here.
