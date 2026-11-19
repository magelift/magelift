# Changelog

Notable changes for MageLift. After the first public release this project follows
[Semantic Versioning](https://semver.org/) and Keep a Changelog. Version bumps
are prepared with Conventional Commits and Release Please.

## [Unreleased]

### Added

- Public repository under `github.com/magelift/magelift` with site at
  [magelift.dev](https://magelift.dev/) (docs under `/docs/`).
- PHP project configuration, environment comparisons, a local Magento quickstart and pull-request previews. Removing the preview label or closing the pull request schedules teardown.
- Account-free provider checks run against Floci for AWS, GCP and Azure. Generated shop workflows deploy the reviewed image digest to the shop’s own account.
- An Azure AKS provider with Pulumi and Floci tests. AWS ECS Fargate and GCP GKE Autopilot have dated Magento acceptance runs. [Provider coverage](docs/capability-matrix.md)
- Imports from Adobe Commerce Cloud, Upsun Flex and Upsun Fixed, including multi-app repositories.
- First-party agent skills under `agents/skills/` for Cursor, Claude Code, and Codex.
- RC contract for the first public tag `v1.0.0-rc.1`; see
  [docs/versioning.md](docs/versioning.md).
- OpenSearch public-tag substitute: offline SigV4 wiring, free-tier
  `searchMode: disabled` matrix, and documented prior Magento+OpenSearch ops
  experience. Live MageLift SigV4 data-plane remains deferred.
- Runtime `/health` short-circuit on nginx and FrankenPHP; brownfield AWS VPC/RDS
  adopt path; PaaS compare and weekend migration guides.

### Notes

- Git history starts at this public import. Older private history is not published.
