## ADDED Requirements

### Requirement: Providers version independently of generic commands

First-party providers MAY ship in this monorepo and MUST still expose version and capability metadata that generic commands can print. Community providers MUST load through the versioned public contract already specified. Independent provider releases MAY be added later; until then, a monorepo release of the core CLI is a valid distribution. Capability detection MUST prefer typed unsupported over stub implementations that claim success.

#### Scenario: Extensions list shows versions

- **WHEN** a user runs the documented extensions or provider list command
- **THEN** each registered provider or integration reports identity, version or module version, and advertised capabilities

#### Scenario: Unsigned remote provider is refused

- **WHEN** RC1 or the current release would load a provider from a remote URL without the documented provenance
- **THEN** loading fails and no provider code is executed
