## Why

Related scope is defined by [`multi-cloud-resilience-observability-edge`](../multi-cloud-resilience-observability-edge/proposal.md). This change owns the public community-extension contract, compatibility, provenance, and clean-cache build proof; the related change owns provider-neutral certification and release evidence requirements.

MageLift can register a community target only by building a custom binary from the MageLift repository, because the current module contract depends on `internal/` packages and a Go Pulumi closure. That keeps the core small, but it is not a usable extension system for an external provider maintainer.

## What Changes

- Define a versioned public provider extension contract separate from internal cloud packages.
- Add explicit extension discovery, compatibility checks, and diagnostics.
- Support an installed community extension without silently downloading or executing unsigned code.
- Preserve compile-time first-party registration for the default binary.
- Add an extension manifest with API version, target IDs, capabilities, executable or module identity, and provenance.
- **BREAKING** Reject extensions whose API version or declared target contract is incompatible with the MageLift binary.

## Capabilities

### New Capabilities

- `provider-extension-loading`: Install, validate, discover, and invoke community provider extensions.

### Modified Capabilities

None. The repository has no existing OpenSpec capability specs.

## Impact

The change affects `sdk/v1`, `internal/platform`, CLI startup and diagnostics, provider registration, release provenance, documentation, and the community provider example. It introduces a security boundary around executable extensions and must not make Pulumi SDKs part of the CLI package layer.
