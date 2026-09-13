## ADDED Requirements

### Requirement: Signed provider download stays digest-pinned

When the published CLI fetches a first-party adapter, it MUST verify checksum and Cosign against `magelift.providers.lock` before exec. Homebrew and Scoop remain the documented install paths after the first public tag.

#### Scenario: Digest mismatch is refused

- **WHEN** a downloaded provider binary does not match the lockfile digest
- **THEN** Magelift does not execute it and reports the provenance failure
