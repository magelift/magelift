# ADR 0008. Verify provider executables before loading

Accepted on 2026-08-22. [ADR 0013](0013-provider-plugin-contract.md) describes
the current plugin layout.

MageLift verifies a provider executable against its lock before starting it
with HashiCorp go-plugin. The lock records the SHA-256 digest and expected
Cosign identity and issuer. A missing signature or a failed verification stops
loading. The working directory supplies a lockfile, never an executable to
load. Compile community modules into an explicit custom binary.
[Distribution trust](0014-distribution-trust.md)

The initial implementation used in-process adapters. The move to separate
provider executables started with [GKE Autopilot](0011-subprocess-dial-proof.md).
