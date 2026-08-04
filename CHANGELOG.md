# Changelog

Notable changes for MageLift. After the first public release this project follows
[Semantic Versioning](https://semver.org/) and Keep a Changelog. Version bumps
are prepared with Conventional Commits and Release Please.

## [1.1.0-rc.1](https://github.com/magelift/magelift/compare/v1.0.0-rc.1...v1.1.0-rc.1) (2026-08-04)


### Features

* host the public site on GitHub Pages ([d0575f4](https://github.com/magelift/magelift/commit/d0575f47c74b38150b0e1c66f1132d87ec40c535))
* publish the CLI to Homebrew and Scoop ([db9ab37](https://github.com/magelift/magelift/commit/db9ab3786e69236529d509207a928684e7782bfa))
* rework the landing page and move the site to website/ ([be8ffac](https://github.com/magelift/magelift/commit/be8ffac489cbc3fc18183789ddc84d661c4a25c1))


### Bug Fixes

* stop stretching the docs hero logo ([247c404](https://github.com/magelift/magelift/commit/247c404d79c3c5a9c8a4d98ae211da9262762ccf))

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
