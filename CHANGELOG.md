# Changelog

All notable changes will be documented here. This project follows Semantic Versioning
after its first public release and uses Conventional Commits with automated release
PRs. The format is based on Keep a Changelog.

## [Unreleased]

### Added

- Initial legal, governance, architecture, documentation, and repository baseline.
- RC contract for first public tag `v1.0.0-rc.1` — see [docs/versioning.md](docs/versioning.md).
- OpenSearch public-tag gate substitute: offline SigV4 wiring mocks, free-tier
  `searchMode: disabled` matrix, and prior Terraform Magento+OpenSearch ops at
  Chantelle ([docs/sources/chantelle-opensearch.md](docs/sources/chantelle-opensearch.md);
  private employer repos not named). Live MageLift SigV4 data-plane deferred.
- Runtime `/health` short-circuit on nginx and FrankenPHP; `curl` in runtime
  images for ECS health checks; Varnish `/health` pass-through test + VCL.

### Changed

- Release readiness gate board: OpenSearch live acceptance is post-tag / paid,
  not a public-tag blocker.
