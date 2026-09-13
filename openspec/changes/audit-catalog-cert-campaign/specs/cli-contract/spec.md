## ADDED Requirements

### Requirement: Cobra stays provider-generic

Root, cleanup, leftover-backup, and other Cobra command files MUST NOT instantiate cloud SDKs. Provider work MUST go through `platform.ModuleRegistry` (and sibling plugin registries). Adding OVH or Scaleway MUST NOT require new client construction in `internal/cli`.

#### Scenario: A new provider is registered

- **WHEN** a first-party Scaleway module is already in the registry
- **THEN** generic cleanup and leftover commands operate through that module without a Scaleway import in the Cobra package
