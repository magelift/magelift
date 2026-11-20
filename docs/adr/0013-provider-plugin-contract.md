# ADR 0013. Provider plugin contract

Accepted on 2026-09-16. The first-party module layout below reflects the
2026-11-19 AWS, Google Cloud, and Azure split.

## Ownership

The CLI evaluates the shop's PHP configuration, resolves its environment,
verifies the provider executable, and coordinates deployment. The provider
owns its cloud API calls, resource graphs, and Pulumi stack operations.
AWS, Google Cloud, and Azure each build an executable from a module under
`providers/<name>/`. Each ships with provider-local tests and a verified
release artifact. [Architecture](../architecture.md)

The core sends provider-specific settings in the target block and shared
settings through typed SDK messages. Providers check region and account
prerequisites during admission. Ordinary planning resolves the intended
target without creating cloud resources.

First-party providers share workload and state helpers from the root Go
module. They pin the root and SDK versions alongside the release. An
explicitly linked community extension can use the public Go SDK.
[Add a provider](../adding-a-provider.md)

## Protocol

HashiCorp go-plugin runs the verified executable as a subprocess. The CLI
checks its digest and Cosign signature before starting it. The handshake and
`Describe` report the provider identity, supported runtimes, protocol version,
and operations. The process runs with the operator's permissions.
[Distribution trust](0014-distribution-trust.md)

Operations cross the boundary as data. A stored plan includes the provider,
runtime, stack name, state backend, image digest, and provider-owned payload.
The core preserves that payload and the provider verifies the identity when
it reads it. The provider runs Pulumi and handles Magento rollout phases.

Required deployment operations are `describe`, `validate-config`, `plan`,
`preview`, `apply`, `outputs`, `destroy`, `state-lock`, `state-unlock`, and
`deploy-app-phase`. Optional operations advertise support through
`Describe`. The CLI reports when the selected target cannot perform an
operation. Secret outputs keep their secret markings.

Put new operation types in `sdk/protocol.go` and related SDK files. Keep cloud
SDK models inside the provider. Add runtime registration and advertised
output keys in `internal/providerhost/standalone.go`.

## Verification

Test the typed exchange using the provider executable and a synthetic shop.
Use Pulumi mocks to check resources and Floci for the APIs it implements.
Run the module checks and `make core-leanness` to confirm the CLI builds
without linking provider implementations. [Test coverage](../capability-matrix.md)
