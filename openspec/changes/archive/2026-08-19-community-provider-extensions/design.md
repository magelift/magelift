## Context

See `proposal.md` for the motivation. The current `platform.StackModule` is internal and returns a Pulumi `RunFunc`, which makes a Go `plugin` ABI or a JSON RPC wrapper a poor first boundary. The repository already supports a compile-time custom binary, but a separate module cannot use that path because of Go `internal/` visibility.

## Goals / Non-Goals

**Goals:**

- Make the existing compile-time extension model usable outside the MageLift repository.
- Keep provider topology and Pulumi SDK imports in the extension implementation.
- Give the default binary an explicit, auditable list of first-party modules.
- Leave a versioned boundary that can support a signed executable protocol later.

**Non-Goals:**

- A Go `plugin` shared-library ABI.
- Automatic download or execution of a community binary in RC1.
- A universal provider YAML catalog.
- Moving cloud topology into `sdk/v1`.

## Decisions

### 1. Publish a public extension SDK and an internal adapter

The public SDK will contain descriptors, typed configuration inputs, target and capability contracts, registration metadata, and optional provider-neutral resilience, edge, and observability ports. Planned adapter factories let an extension construct its SDK clients from its opaque plan without importing `internal/`; MageLift's internal adapter will translate the public registration into the existing `ModuleRegistry` and day-2 ports. The public package must not depend on `internal/`.

Alternative rejected: exporting `internal/platform` directly. It would freeze the CLI's internal orchestration types as the community API and expose Pulumi lifecycle details that are still evolving.

### 2. Keep RC1 loading compile-time and explicit

Community providers will ship as Go modules and custom binaries that call an exported registration function. The extension manifest and diagnostic surface make the loaded set visible. Remote executable loading remains a later change after a signed process protocol is designed and tested.

Alternative rejected: Go's `plugin` package. It is not a portable distribution boundary for MageLift's release targets and does not solve trust or versioning.

### 3. Keep generic behavior in the core and native behavior in adapters

The core validates portable intents, ownership, idempotency, recovery graphs,
certification evidence, and cleanup. A provider adapter translates those
contracts to its native SDK and returns explicit capability or unsupported
errors; it must not duplicate core policy or silently no-op. Provider-specific
options remain in an extension-owned namespace, and a community adapter uses
the same SDK ports as a first-party adapter.

The first-release portable schema intentionally has no singular
`edge.provider` or `observability.provider` field. Composable native and
external boundaries use `nativeProvider` and `externalProvider`; `provider` on
target/resource identity is still valid identity data.

### 4. Treat the manifest as metadata, not executable configuration

The manifest declares API version, extension identity, targets, capabilities, certification tier, and provenance. It cannot contain arbitrary Pulumi or shell instructions. Provider code remains compiled and reviewed by the extension maintainer.

### 5. Test a clean-cache custom binary

The provider example will remain the contract test. It will build from an empty Go module and build cache, print its extension diagnostics, and reject a deliberately incompatible descriptor.

## Risks / Trade-offs

- [The public contract is too narrow for a provider] -> Version additive descriptors and keep provider-specific options in extension-owned namespaces.
- [The contract leaks unstable Pulumi types] -> Keep Pulumi program ownership in the extension adapter and expose only the lifecycle boundary required by the core.
- [A provider duplicates core recovery policy] -> Validate SDK descriptors at module registration and keep scheduling, evidence, and cleanup in the core.
- [Community binaries remain difficult to distribute] -> Add signed executable loading only after the RC1 contract and trust policy are stable.

## Migration Plan

1. Extract public descriptors and registration types without changing current first-party modules.
2. Add an internal adapter and migrate the existing custom CLI example.
3. Add contract and clean-cache tests.
4. Publish extension authoring and certification-tier documentation.
5. Keep current compile-time registration as the fallback while community modules migrate.
