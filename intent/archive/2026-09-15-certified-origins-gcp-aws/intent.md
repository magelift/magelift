---
status: accepted
slug: certified-origins-gcp-aws
---

# Intent: two certified origins proved on a tight testing budget

## Problem

Agencies need at least two production-capable origins to trust the multi-cloud claim, but live testing costs real money: about $180 of AWS free credits remain, while the GCP project has effectively unlimited budget. Proving everything live on every provider would drain the AWS credits and the maintainer's credit card before v1 ships.

## Evidence

Capability matrix: AWS ECS Fargate preview tuple and evidenced GCP GKE Autopilot cells are certified; EKS, GKE Standard, MKS, Kapsule are experimental. Evidence pack (`docs/evidence/README.md`) holds the current proofs: AWS `20260813ai`, GCP `gcap28`. `openspec/specs/efficient-cloud-certification/spec.md` already requires the pyramid: unit, mocks, Floci, then packed live KEEP sessions, GCP first, integrations attached to the GCP origin, AWS session skipping already-proven vendors. `make local-gates` runs Pulumi mocks plus harness plus Floci AWS plus floci-gcp with no account. Measured AWS preview create ~10m31s, GCP preview infra-only ~19m21s.

## Proposed outcome

GCP GKE Autopilot is the reference origin: preview certified plus the minimal standard/HA hardening v1 promises, all proved in packed sessions with destroy plus `assert_clean`. AWS ECS Fargate is the second certified origin proved in one packed session: create-once plus warm catalog transitions on one digest, minimum viable sizes, short-lived stacks. Every cell that Floci, Pulumi mocks, or unit tests can close is closed before any cloud spend. AWS spend stays inside the remaining credits with a written budget per session. OVH/Scaleway stay out of this intent.

## Affected users and systems

Magento shops targeting GCP or AWS. `internal/cloud/gcp`, `internal/cloud/aws`, `internal/cloud/kube`, certification scheduler and seal/verify commands, `tests/floci`, `tests/floci-gcp`, acceptance scripts, capability matrix, evidence pack.

## Constraints

GCP first; AWS second and never re-proving Fastly, New Relic, SendGrid, or Cloudflare. Destroy on exit always; no KEEP except a named packed GCP session with a written reason. Thin-credit discipline: minimum sizes, short-lived Magento, ownership tags, explicit cleanup, spend recorded per session. Emulators never certify Autopilot, Armor data plane, managed TLS, Cloud SQL PITR, live WAF, OIDC token exchange, or regional DR. Mocks alone never certify a cell.

## Out of scope

Search data-plane (see `magento-search-v1`), EKS, GKE Standard certification, OVH/Scaleway Magento, X-Ray, regional DR.

## Open questions

Which exact standard/HA cells must be live-proved for v1 versus documented experimental? What is the per-session AWS dollar cap that keeps total spend inside the remaining credits with margin for one retry?
