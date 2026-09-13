## ADDED Requirements

### Requirement: Published CLI installs signed provider artifacts

The published RC1 CLI MUST ship without bundling every first-party cloud SDK. Homebrew and Scoop MUST install the core binary. When a command needs a provider or integration, the CLI MUST consult `magelift.providers.lock`, download the matching first-party artifact from the documented registry, verify Cosign (or documented equivalent) signatures and checksums, and refuse unsigned, checksum-mismatched, or digest-unpinned artifacts. In-process linking of first-party adapters MUST remain valid for tests and as a documented one-release ceiling if subprocess extract lags the RC1 tag.

#### Scenario: GCP-only project fetches one adapter

- **WHEN** a user installs the core CLI and runs a command against `target.provider: gcp` with an empty provider cache
- **THEN** MageLift installs only the signed `magelift-provider-gcp` artifact listed in the lockfile and does not download OVH, Scaleway, or AWS adapters

#### Scenario: Unsigned provider asset is refused

- **WHEN** a provider artifact is missing a Cosign signature or its digest does not match the lockfile
- **THEN** the CLI refuses to execute it and reports the provenance failure without running provider code

## MODIFIED Requirements

### Requirement: Provider packages follow the monorepo until a split is specified

First-party providers and integrations MUST live in this monorepo as separately versionable packages. They MUST carry their own versions and satisfy `provider-extension-loading`. Community providers MUST NOT be required to live in this repository. Changelog and GitHub release notes MUST list user-visible CLI and YAML changes for the core module separately from adapter versions.

#### Scenario: A community provider is not in the core tarball

- **WHEN** a user installs only the core MageLift binary
- **THEN** an uninstalled first-party or community provider is reported as unregistered with the documented install command rather than silently loaded from the network
