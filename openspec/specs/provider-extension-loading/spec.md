## Purpose

Gives community maintainers a versioned way to add MageLift targets and capabilities without importing MageLift's internal packages or changing the default binary.

## Requirements

### Requirement: Versioned public contract

Community extensions MUST implement a public, versioned contract that does not require imports from `internal/cli`, `internal/platform`, or `internal/cloud`.

#### Scenario: External module builds from a clean module cache

- **WHEN** a community provider follows the published extension example
- **THEN** its custom MageLift binary builds without importing an internal package from a separate module path

### Requirement: Explicit registration

An extension MUST be registered explicitly by a custom binary or an explicitly selected extension bundle. The default MageLift binary MUST NOT discover and execute arbitrary files from the working directory.

#### Scenario: Unregistered provider appears in YAML

- **WHEN** a project selects a provider that is not registered
- **THEN** MageLift reports the provider and the required extension instead of executing a nearby binary

### Requirement: Contract and target validation

The extension loader MUST validate API version, target IDs, provider IDs, capability IDs, required output keys, and declared certification tier before making the target available.

#### Scenario: Extension targets an incompatible API

- **WHEN** an extension declares an unsupported API version
- **THEN** startup fails with an actionable compatibility error and does not register the target

### Requirement: Provenance metadata

Every installed or compiled extension MUST expose its module version, source identity, target IDs, and build provenance in diagnostics.

#### Scenario: User lists installed extensions

- **WHEN** the user runs the extension diagnostic command
- **THEN** MageLift prints the extension identity, API version, targets, and provenance without exposing credentials

### Requirement: Dual-mode module loading

MageLift MUST load first-party and community modules through the same versioned public contract. Tests, Floci suites, and acceptance harnesses MUST be able to load a module in-process. The published CLI MUST load a module as a signed subprocess that speaks that contract over HashiCorp go-plugin (gRPC). MageLift MUST NOT use Go `plugin.Open` or execute unsigned files from the working directory.

#### Scenario: Tests stay in-process

- **WHEN** `go test` or `make floci-test-aws` exercises a first-party module
- **THEN** the module runs in-process through the public SDK contract and does not require a downloaded provider binary

#### Scenario: Published CLI uses a subprocess

- **WHEN** a released `magelift` binary plans or deploys a registered provider whose artifact is installed and verified
- **THEN** provider code runs in a HashiCorp go-plugin subprocess whose identity matches the lockfile digest and advertised SDK API version

### Requirement: No unsigned remote execution in RC1

The RC1 extension path MUST NOT download or execute an unsigned remote provider. Signed, digest-pinned first-party artifacts from the documented registry, and community artifacts that satisfy the documented trust policy, Cosign verification, and lockfile digest, MAY be installed. Any executable plugin path MUST require that trust policy and signature verification.

#### Scenario: Remote extension has no trusted provenance

- **WHEN** a remote extension lacks a configured trust identity or digest
- **THEN** MageLift refuses to install or execute it

#### Scenario: Signed first-party adapter is installed

- **WHEN** the lockfile names a first-party provider digest that Cosign verifies from the documented registry
- **THEN** MageLift MAY install and execute that subprocess and MUST still refuse a matching filename that fails verification

### Requirement: Providers version independently of generic commands

First-party providers MUST ship in this monorepo and MUST expose version and capability metadata that generic commands can print. Community providers MUST load through the versioned public contract already specified. Independent provider releases MUST be allowed without a core CLI version bump when the SDK API version still matches. Capability detection MUST prefer typed unsupported over stub implementations that claim success.

#### Scenario: Extensions list shows versions

- **WHEN** a user runs the documented extensions or provider list command
- **THEN** each registered provider or integration reports identity, version or module version, and advertised capabilities

#### Scenario: Unsigned remote provider is refused

- **WHEN** RC1 or the current release would load a provider from a remote URL without the documented provenance
- **THEN** loading fails and no provider code is executed

#### Scenario: Adapter release without core bump

- **WHEN** a first-party adapter tag increments while the core CLI and SDK API version remain compatible
- **THEN** `magelift` accepts the new adapter version from the lockfile without requiring a new core tag

### Requirement: Lean core and community catalog are P2

The published CLI MUST be able to load signed, digest-pinned provider artifacts without bundling every cloud SDK. Community providers MUST install only when Cosign identity, issuer, and lockfile digest match. Magento deploy MUST stay in-process until Dial is proven. Unsigned remote execution MUST remain refused.

#### Scenario: Unsigned community plugin is refused

- **WHEN** a user would load a remote provider without Cosign identity and lock digest
- **THEN** Magelift refuses before any provider code runs
