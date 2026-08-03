# Changelog

All notable changes will be documented here. This project follows Semantic Versioning
after its first public release and uses Conventional Commits with automated release
PRs. The format is based on Keep a Changelog.

## [1.0.1-rc.1](https://github.com/acourtiol/magelift/compare/v1.0.0-rc.1...v1.0.1-rc.1) (2026-08-03)


### Bug Fixes

* sample-shop onboarding docs, usererr tests, and v* tags ([94028cf](https://github.com/acourtiol/magelift/commit/94028cf48fad84d6ba88a80c4787af6ec4a6d58c))

## 1.0.0-rc.1 (2026-08-03)


### Features

* community launch docs, marketing scaffold, and install path ([c1c0df2](https://github.com/acourtiol/magelift/commit/c1c0df2e57cc761659d7c57e970520cb33ad2913))


### Bug Fixes

* Dependabot cooldowns and docs workflow Python setup ([a1a61f0](https://github.com/acourtiol/magelift/commit/a1a61f0f138019f96e82f53932e792ad6625a798))
* drive Release Please from config (RC.1) ([d3a729b](https://github.com/acourtiol/magelift/commit/d3a729b43f1717d432dec1a4d9635a5e080b9d20))
* green CI paths for docs, zizmor, and marketing lockfile ([8358541](https://github.com/acourtiol/magelift/commit/835854152f12b155132f9a5e848521c17f0f8ba1))

## [Unreleased]

### Added

- Certify `gcp` / `gke-autopilot` after real-account acceptance — multi-cloud claim for two first-party targets (account IDs redacted in public evidence).
- Live free-tier AWS brownfield adopt confirm (existing VPC + RDS; describe-after-destroy).
- Community Launch publishing decisions ([docs/publishing.md](docs/publishing.md)), committed acceptance evidence samples, MkDocs Pages workflow, and marketing site scaffold.
- Cost vs PaaS comparison page and weekend migration packaging for evaluators.
- Initial legal, governance, architecture, documentation, and repository baseline.
- RC contract for first public tag `v1.0.0-rc.1` — see [docs/versioning.md](docs/versioning.md).
- OpenSearch public-tag gate substitute: offline SigV4 wiring mocks, free-tier
  `searchMode: disabled` matrix, and prior Terraform Magento+OpenSearch ops at
  Chantelle ([docs/sources/chantelle-opensearch.md](docs/sources/chantelle-opensearch.md);
  private employer repos not named). Live MageLift SigV4 data-plane deferred.
- Runtime `/health` short-circuit on nginx and FrankenPHP; `curl` in runtime
  images for ECS health checks; Varnish `/health` pass-through test + VCL.

### Fixed

- Accept RDS ManageMasterUserPassword secret ARNs that contain `!` (`rds!db-…`) when adopting an existing database.

### Changed

- Public git history replaced with a clean import; planner artifacts and personal cloud account identifiers are not published.
- Release readiness gate board: OpenSearch live acceptance is post-tag / paid,
  not a public-tag blocker.
- README / docs: GCP GKE Autopilot marked certified beside AWS ECS Fargate.
