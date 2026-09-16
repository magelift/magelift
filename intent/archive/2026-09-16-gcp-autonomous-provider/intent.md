---
status: done
slug: gcp-autonomous-provider
---
# Intent: autonomous GCP provider (the first real plugin)

## Problem

No provider is autonomous today. The GCP Autopilot subprocess path is a narrow
proof that falls back to in-process execution on load or dial errors with no
YAML-driven download, day-two operations stay host-owned, and GCP operations
can depend on a saved token that expires about an hour after deploy. A plugin
that provisions but cannot independently operate — and that silently changes
implementation on verification failure — is not the product vision.

## Evidence

`intent/audit.md` F02: `internal/cli/subprocess.go` (`defaultNewBackend`,
`subprocessBackend`) serves only `gcp`/`gke-autopilot` from a subprocess when
artifacts sit beside the CLI, and "a failed subprocess attempt never fails the
command: it falls back with a stderr notice". There is no complete YAML-driven
provider download path. A rejected checksum or incompatible plugin changes the
selected implementation instead of failing closed (execution-path
substitution, not proof of unverified-binary execution).

F01 provider half: the shipped registration path still compiles providers
in-process; the autonomous provider must prove the contract's separation with
a build outside the root module using only the public SDK.

F04: `internal/cloud/gcp/runtime/runtime.go` (`generateKubeconfig`) builds
kubeconfig from `GetClientConfigOutput().AccessToken`, and
`internal/cloud/kube/client.go` reads stack-output credentials unless
`MAGELIFT_KUBECONFIG` overrides — with its own comment noting the roughly
one-hour expiry. Result: successful deploy, failed operations an hour later.
A manual kubeconfig override does not satisfy the YAML-only path.

## Proposed outcome

One autonomous GCP Autopilot provider built outside the root module against
the approved contract: it provisions and independently operates the alpha
recipe (status, logs, execution, backup, restore, teardown, credential
refresh) over the versioned protocol with explicit compatibility negotiation
and capabilities. Integrity or compatibility failure fails closed — no silent
fallback to embedded execution (an intentional migration mode may exist but
never silently converts plugin errors into in-process runs). Operations obtain
fresh credentials through the provider's auth mechanism; expiry, refresh
failure, restart, and CI identity are tested explicitly, and refreshed secrets
never land in evidence. The core builds without provider SDK dependencies.

## Affected users and systems

Alpha pilot shops on GCP. The new provider module home (decided by
`provider-plugin-contract`), `internal/providerhost/` (client side),
core registration and subprocess loading, GCP runtime/ops/edge/observability
adapters as moved, `docs/capability-matrix.md` GCP rows, evidence pack.

## Constraints

- Implements the approved `provider-plugin-contract` spec and ADR; no
  freelancing the protocol.
- Fail closed on integrity or compatibility failure; no silent fallback.
- YAML-only operating path: fresh credentials automatically, no manual
  kubeconfig as the documented flow.
- No secret values in YAML, logs, or evidence — references only; refreshed
  credentials included.
- Topology ownership rules from the contract hold; no shared components
  behind `if provider ==`.
- Tests ship with the implementation: negotiation, capability mismatch,
  checksum rejection, expiry/refresh/restart/CI-identity, and each day-2
  operation.
- Human docs touched here go through humanizer, then remove-ai-marks.

## Out of scope

- AWS, OVH, Scaleway providers (AWS: `aws-provider-parity` post-alpha; EU:
  deferred).
- Distribution mechanics (download, install trust, module publication):
  specified by the contract, implemented in `verified-provider-distribution`;
  this intent proves the provider, distribution proves the delivery.
- Magento lifecycle semantics (owned by `magento-deployment-safety`; this
  provider consumes the fixed behavior).
- The alpha recipe docs and onboarding (owned by
  `full-deployment-coverage`).

## Open questions

- Exact provider module home and import path under the contract's decision
  (e.g. nested module vs `providers/gcp`)? Default: whatever the approved
  contract names. Owner: contract spec author.
- GCP is the first autonomous provider because existing functional search and
  operator evidence minimizes new proof. If pilot interviews demand AWS
  first, the order flips at intent acceptance with identical gates — but only
  one reference implementation at a time. Owner: maintainer at acceptance.
- Which day-2 operations need live proof in this intent vs deferred to
  `reference-store-acceptance`? Default: unit plus fake-client proof here for
  all seven, live proof of the full loop in acceptance. Owner: spec author.
