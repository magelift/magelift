# ADR 0002: Certified vs experimental, evidence is authority

- Status: Accepted
- Date: 2026-08-22

## Context

The tree ships more adapters than MageLift will stand behind in production. Calling every in-tree runtime "certified" would overclaim. Calling only AWS certified after GCP Autopilot passed would underclaim.

## Decision

A cell is **certified** only when `docs/capability-matrix.md` and the current file in `docs/evidence/` say so. Today that is:

- AWS ECS Fargate
- GCP GKE Autopilot (evidenced runtime cells)

AWS EKS, GCP GKE Standard, OVH MKS, and Scaleway Kapsule are **experimental**. They may change or return `ErrNotSupported` on day-2 commands. Community compile-in providers are never MageLift-certified.

Evidence, not a descriptor string, is the authority.

Adobe-unsupported combinations fail before mutate unless `compatibility.allowUnsupported` records that Adobe hatch. The hatch does not recertify a cell and does not apply to MageLift-experimental or provider-unavailable targets.

MageLift-experimental cells (EKS, GKE Standard, OVH MKS, Scaleway Kapsule, dense AWS catalog such as Aurora or Amazon MQ) warn and proceed. The warning names MageLift. They are never silent and never certified.

Provider-unavailable cells (no adapter or SKU) still fail closed.

`nginx-fpm` is the Adobe-aligned default. `frankenphp-classic` and `php-apache` are Adobe-unsupported plugins that require `compatibility.allowUnsupported` on the current Magento line. Adobe has no row for either plugin, neither is certified, and `frankenphp-worker` remains unregistered.

## Consequences

CLI, README, and the public site must match the matrix. A live pass on one Magento 2.4.9 shape does not certify every release, edge vendor, or HA profile.

## Alternatives considered

- Certify every in-tree adapter: rejected. Evidence is incomplete.
- AWS-only certified forever: rejected once Autopilot passed the Magento suite.

## Provenance

`docs/capability-matrix.md` and `docs/evidence/README.md`.
