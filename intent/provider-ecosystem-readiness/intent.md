---
status: draft
slug: provider-ecosystem-readiness
---

# Intent: provider ecosystem readiness

## Problem

Two working first-party providers are necessary but not sufficient evidence
that MageLift has a durable provider platform. Alpha lockstep versions,
root-internal imports, core-owned provider configuration knowledge, and
first-party trust assumptions may still make a third provider require edits
throughout the core.

Freezing the SDK before GCP and AWS expose their real common needs would lock in
accidents. Adding another cloud before hardening the proven boundary would copy
them.

## Evidence

[The provider plugin ADR](../../docs/adr/0013-provider-plugin-contract.md)
defines the intended ownership, typed operations, provider-owned configuration,
negotiation, and post-alpha independent releases.

[The architecture report](../report.md) recommends stabilizing the extension
boundary only after GCP and AWS are supported. The GCP and AWS intents provide
the two concrete consumers needed to distinguish portable contracts from
provider-specific implementation.

## Proposed outcome

The shipped core has no cloud provider SDK or Pulumi provider dependencies.
First-party provider modules import only the public SDK and explicitly
published support modules. Provider-specific configuration schemas and
validation are provider-owned while the core retains the stable YAML envelope.

A provider conformance kit proves protocol negotiation, required operations,
cancellation, progress, typed errors, config validation, artifact handling,
process cleanup, trust, and contract compatibility without cloud credentials.

Version-compatibility policy permits a provider release independent of the core
when the negotiated contract allows it. Adding a third provider has a
documented scaffold and requires no core source edits except an explicit trust
or catalog decision.

## Affected users and systems

Provider authors; SDK consumers; core CLI dependency closure; provider modules;
configuration schema generation; release and compatibility policy;
conformance tests and contributor documentation.

## Constraints

- Begins only after both GCP and AWS autonomous providers pass their product
  paths.
- Do not design for hypothetical clouds where two proven providers give no
  evidence.
- Keep the common YAML envelope stable and provider topology opaque to core.
- Process isolation is a compatibility boundary, not a security sandbox.
- First-party trust does not automatically authorize third-party publishers.
- Public contract, ADR, docs, examples, and conformance tests change together.
- No stable-v1 promise is made until pilot evidence supports it.

## Out of scope

- Implementing a third cloud provider.
- A public marketplace or arbitrary remote-code execution.
- Cross-provider Pulumi components.
- Stable v1 release.
- Provider-specific product features.

## Open questions

- Is the first external extension model compile-time custom binaries, a curated
  signed catalog, or first-party providers only? Decide after the GCP/AWS trust
  and support costs are measured; this intent must not silently broaden trust.
