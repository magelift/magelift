---
status: draft
slug: aws-provider-parity
---
# Intent: AWS provider parity (second plugin, same protocol, post-alpha)

## Problem

DRAFT for post-alpha sequencing; not on the alpha path. After the alpha
proves one autonomous GCP provider on the public protocol, AWS ECS must follow
through the same protocol — not as a second bespoke core coupling. The AWS
side additionally carries onboarding gaps (certificates, notification topic,
logging inputs) and search evidence that today proves infrastructure without
the full application-level sequence GCP already demonstrated.

## Evidence

`intent/audit.md` F13 AWS half: existing AWS onboarding exposes certificate,
notification, and logging prerequisites needing explicit treatment; proposed
follow-up slugs in the old deployment audit (`acm-managed-certificates`,
`managed-notify-topic`) never became intents. The partially implemented SES
files must be deliberately completed, isolated, or removed.

F08 AWS half: the AWS search report records infrastructure success but
explicitly lacks the full application-level sequence (reindex, HTTP product
result, post-recycle querying) the GCP report contains. Do not promote AWS
from infrastructure proof.

F05 AWS half: ECS stabilization/health repair lands in
`magento-deployment-safety`; this intent consumes it on the AWS path.

Budget context from the old roadmap (verify at resume time): AWS live work
was capped inside remaining credits (~$180 at last accounting) with minimum
sizes, short-lived stacks, and retry margin. Re-verify the budget before any
live AWS session; do not inherit a stale number.

## Proposed outcome

After alpha: an autonomous AWS ECS provider built outside the root module
against the same approved contract and distribution mechanics as GCP, proving
the protocol is not GCP-shaped. AWS onboarding gaps are closed or honestly
documented (managed ACM with DNS validation, managed notify topic, logging
inputs, SES disposition), and AWS search earns application-level proof or
stays honestly infrastructure-only. Stable v1 freezes only contracts supported
by both providers plus pilot experience.

## Affected users and systems

AWS pilot shops. The AWS provider module home (per the contract), AWS
adapters (stack, runtime, edge, observability, search, cost), capability
matrix AWS rows, evidence pack, AWS onboarding docs.

## Constraints

- Same public protocol, same distribution trust policy, same fail-closed
  rules as GCP. No AWS-specific core coupling.
- Live AWS work stays inside a written per-session cap with retry margin;
  minimum sizes, short-lived stacks, destroy plus `assert_clean` always.
- No Aurora or Amazon MQ live unless the spec justifies the spend against
  pilot demand.
- Honest search labeling until application-level proof lands.
- Human docs touched here go through humanizer, then remove-ai-marks.

## Out of scope

- The alpha tag or any alpha gate: this intent starts after
  `reference-store-acceptance` tags alpha and pilot feedback begins.
- GCP changes except protocol conformance fixes both providers need.
- EU providers (separately deferred).
- Regional DR, X-Ray, three-node HA search, or provider cost adapters beyond
  what the spec justifies.

## Open questions

- Managed vs documented-manual for each AWS gap (ACM certs, SNS topic, log
  bucket, SES identity/credentials/sandbox)? Default: managed where Pulumi
  coverage is stable and support cost is bounded; honest procedures otherwise.
  Owner: spec author at resume time.
- AWS recipe pins and scale bounds mirrored from the alpha GCP recipe, or
  AWS-native choices? Default: mirror where sensible, diverge only with
  evidence. Owner: spec author.
- Live budget at resume time (credits remaining, per-session cap, retry
  margin)? Default: re-verify, never inherit. Owner: maintainer.
