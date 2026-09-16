---
status: deferred
slug: eu-providers-experimental
---
# Intent: EU providers experimental (DEFERRED past alpha)

## Problem

DEFERRED 2026-09-16 by `intent/audit.md`: neither OVH nor Scaleway is
necessary to prove the first supported release. The alpha proves one complete
GCP Autopilot path; breadth before that proof multiplies upgrade, recovery,
compatibility, and support obligations without validating that anyone trusts
the product with a storefront. EU work resumes after alpha on the same public
provider protocol — it does not gate the alpha tag.

## Evidence

Prior `plan.md` in this directory: Scaleway live refresh GREEN (`scw915`
nl-ams-1, exit 0, <€1 inside cap); OVH PARKED 2026-09-15 after 6 attempts, 0
PASS (MKS pools stuck INSTALLING, flavors 404, MIL inconclusive). August OVH
evidence stands; retry is another-day work per maintainer call, not an alpha
gate.

Capability matrix: EU certified subsets stay empty; every cell experimental.
Live proof is infrastructure-only; Magento runtime is `not-run`. Retained, not
erased.

## Proposed outcome

After alpha: OVH MKS and Scaleway Kapsule return as explicit experimental
adapters on the public provider protocol, with refreshed infra-only live proof
as needed, unavailable cells failing closed with the exact boundary named, and
day-2 gaps as typed `ErrNotSupported`. Magento certification on either waits
for post-alpha budget and demand. EU shops can evaluate on real EU
infrastructure with no false production promise.

## Affected users and systems

EU shops evaluating non-US clouds. `internal/cloud/ovh`,
`internal/cloud/scaleway` (or their post-contract homes), capability matrix EU
rows, evidence pack, EU experimental docs.

## Constraints

- Shared own-money cap discipline holds whenever live EU work resumes:
  serialized, destroy on exit, no KEEP, minimum SKUs, six-hour-or-shorter TTL.
- Magento stays `not-run`/experimental until a Magento-compatible digest is
  exercised live.
- Post-contract, EU adapters follow the same public protocol as GCP/AWS; no
  bespoke core coupling.

## Out of scope

- Magento certification on either provider, HA, managed search on EU
  providers, Redis cluster mode until the connector carries discovery
  endpoints, OVH/Scaleway database attach.
- Any alpha gate: this intent cannot block `reference-store-acceptance`.

## Open questions

- When pools converge, which single preview shape per provider is the
  documented evaluation path (Scaleway nl-ams-1 shape held; OVH PAR vs MIL)?
  Owner: maintainer at resume time.
- Does EU resume before or after `aws-provider-parity`? Default: after AWS
  parity, unless a pilot pays for EU first. Owner: maintainer.
