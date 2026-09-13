---
status: draft
slug: eu-providers-experimental
---

# Intent: honest EU provider path without draining the card

## Problem

EU companies ask for Magento on EU cloud providers, and Scaleway plus OVH are pay-as-you-use on the maintainer's credit card. Full Magento certification on both before v1 would cost money the project does not have and delay the stable cut. The alternative to slow, honest experimental support is claiming support that was never proved.

## Evidence

Capability matrix: `certification-ovh` and `certification-scaleway` certified subsets are empty; every cell experimental. OVH Bootstrap/Secrets return `ErrNotSupported`; Scaleway Bootstrap returns `ErrNotSupported`, cache family is Redis not Valkey. Live proof is infrastructure-only (`ovh-mks-infrastructure-live-2026-08-12`, `scaleway-kapsule-infrastructure-live-2026-08-13`); Magento runtime is `not-run`. `openspec` caps live Scaleway plus vendor cells at a shared $50 own-money budget, one serialized preview each, no KEEP, destroy always. EU residency interest from companies: stated by maintainer, individual requests not checked in this session.

## Proposed outcome

For v1, OVH MKS and Scaleway Kapsule ship as complete first-party adapters with Magento explicitly experimental: infra-only live proof refreshed as needed, unavailable cells fail closed with the exact boundary named (e.g. Scaleway Redis versus Valkey, OVH native CDN), day-2 gaps stay typed `ErrNotSupported` instead of stubs. One serialized destroy-on-exit preview per provider proves the adapter still applies; Magento certification waits for post-v1 budget. EU shops can evaluate on real EU infrastructure with no false production promise.

## Affected users and systems

EU shops evaluating non-US clouds. `internal/cloud/ovh`, `internal/cloud/scaleway`, shared `internal/cloud/kube` observe/steps, capability matrix EU rows, evidence pack, `docs/ovh-experimental.md`, `docs/scaleway-experimental.md`.

## Constraints

Shared $50 own-money cap for Scaleway plus vendor cells; equivalent restraint for OVH. One preview at a time per provider, serialized, destroy on exit, no KEEP. Minimum SKUs (OVH essential/discovery, smallest viable Kapsule/RDB/Redis). Magento stays `not-run`/experimental until a Magento-compatible digest is exercised live. Account-free cost capacity stays; live provider pricing stays unwired until verified.

## Out of scope

Magento certification on either provider, HA, managed search on EU providers, Redis cluster mode until the Magento connector carries discovery endpoints, OVH/Scaleway database attach.

## Open questions

Do we refresh infra-only live proof for both providers before v1, or is the August 2026 evidence plus mock/Floci coverage enough for an experimental label? Which single preview shape per provider is the documented evaluation path?
