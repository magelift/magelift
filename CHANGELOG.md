# Changelog

Notable changes for MageLift. After the first public release this project follows
[Semantic Versioning](https://semver.org/) and Keep a Changelog. Version bumps
are prepared with Conventional Commits and Release Please.

## [Unreleased]

### Added

- Public repository under `github.com/magelift/magelift` with site at
  [magelift.dev](https://magelift.dev/) (docs under `/docs/`).
- Certified targets: AWS ECS Fargate and GCP GKE Autopilot (acceptance evidence
  under `docs/evidence/`).
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
