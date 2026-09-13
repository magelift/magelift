## Purpose

Gives community maintainers a versioned way to add MageLift targets and capabilities without importing MageLift's internal packages or changing the default binary.

## ADDED Requirements

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

### Requirement: No unsigned remote execution in RC1

The RC1 extension path MUST NOT download or execute an unsigned remote provider. Any future executable plugin path MUST require an explicit trust policy and signature verification.

#### Scenario: Remote extension has no trusted provenance

- **WHEN** a remote extension lacks a configured trust identity or digest
- **THEN** MageLift refuses to install or execute it
