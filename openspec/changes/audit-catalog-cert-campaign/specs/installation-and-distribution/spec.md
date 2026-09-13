## ADDED Requirements

### Requirement: First-party provider artifacts are releasable

GoReleaser and CI MUST be able to publish signed, digest-pinned first-party provider binaries that the lean CLI can Dial. A `cmd/magelift-provider-*` that only prints a kind string MUST NOT be treated as a runnable plugin. Homebrew/Scoop stay after the first public core tag; they are not this campaign's Magento cells.

#### Scenario: Provider binary is missing from the release matrix

- **WHEN** RC1 would claim subprocess providers
- **THEN** the documented provider artifact is in the release matrix and Cosign-verified, or the claim stays unimplemented
