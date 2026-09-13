## Purpose

Specifies how MageLift and provider packages are built, signed, distributed, installed, and upgraded across supported operating systems.

## Requirements

### Requirement: Release artifacts are signed binaries

Stable and RC releases MUST publish binaries for linux, darwin, and windows on amd64 and arm64, with checksums and Cosign (or documented equivalent) signatures. `magelift version` MUST print the released version. `magelift upgrade` MUST use the documented upgrade path or fail closed if the install method cannot self-update. Unsigned or checksum-mismatched artifacts MUST NOT be installed by MageLift-owned upgrade paths.

#### Scenario: Version after a release install

- **WHEN** a user installs a published release and runs `magelift version`
- **THEN** the output includes the tag version and does not report a dirty source-build identity unless they installed from source

### Requirement: Homebrew and Scoop are first-class install paths

Distribution MUST include a Homebrew cask (not a deprecated formula) and a Scoop manifest, published to the documented taps or buckets when release tokens are present. Until the first public tag, docs MUST describe source install (`go install` or a release archive) as the supported path and MUST NOT claim brew or scoop install works.

#### Scenario: Homebrew cask after the first tag

- **WHEN** `v1.0.0-rc.1` or later is published with tap credentials
- **THEN** the Homebrew cask installs the matching signed binary and `magelift version` matches the tag

### Requirement: Published CLI installs signed provider artifacts

The published RC1 CLI MUST ship without bundling every first-party cloud SDK. Homebrew and Scoop MUST install the core binary. When a command needs a provider or integration, the CLI MUST consult `magelift.providers.lock`, download the matching first-party artifact from the documented registry, verify Cosign (or documented equivalent) signatures and checksums, and refuse unsigned, checksum-mismatched, or digest-unpinned artifacts. In-process linking of first-party adapters MUST remain valid for tests and as a documented one-release ceiling if subprocess extract lags the RC1 tag.

#### Scenario: GCP-only project fetches one adapter

- **WHEN** a user installs the core CLI and runs a command against `target.provider: gcp` with an empty provider cache
- **THEN** MageLift installs only the signed `magelift-provider-gcp` artifact listed in the lockfile and does not download OVH, Scaleway, or AWS adapters

#### Scenario: Unsigned provider asset is refused

- **WHEN** a provider artifact is missing a Cosign signature or its digest does not match the lockfile
- **THEN** the CLI refuses to execute it and reports the provenance failure without running provider code

### Requirement: Provider packages follow the monorepo until a split is specified

First-party providers and integrations MUST live in this monorepo as separately versionable packages. They MUST carry their own versions and satisfy `provider-extension-loading`. Community providers MUST NOT be required to live in this repository. Changelog and GitHub release notes MUST list user-visible CLI and YAML changes for the core module separately from adapter versions.

#### Scenario: A community provider is not in the core tarball

- **WHEN** a user installs only the core MageLift binary
- **THEN** an uninstalled first-party or community provider is reported as unregistered with the documented install command rather than silently loaded from the network

### Requirement: Signed provider download stays digest-pinned

When the published CLI fetches a first-party adapter, it MUST verify checksum and Cosign against `magelift.providers.lock` before exec. Homebrew and Scoop remain the documented install paths after the first public tag.

#### Scenario: Digest mismatch is refused

- **WHEN** a downloaded provider binary does not match the lockfile digest
- **THEN** Magelift does not execute it and reports the provenance failure
