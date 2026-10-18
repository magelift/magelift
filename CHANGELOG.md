# Changelog

Notable changes for MageLift. After the first public release this project follows
[Semantic Versioning](https://semver.org/) and Keep a Changelog. Version bumps
are prepared with Conventional Commits and Release Please.

## 1.0.0 (2026-10-18)


### Features

* add experimental cloud and SaaS adapters ([03567ca](https://github.com/magelift/magelift/commit/03567ca247aea49683fb9f2bbe924ce7fed69475))
* add the YAML contract, SDK, and CLI ([0546cb6](https://github.com/magelift/magelift/commit/0546cb6b935b0398b9f5015a449c3b263ee34dd4))
* **aws:** add the ECS Fargate provider ([40ffe7f](https://github.com/magelift/magelift/commit/40ffe7f06cd859c53fa8e1a6bac860fca2d36235))
* **build:** add Magento image build and local runtime ([1d3829b](https://github.com/magelift/magelift/commit/1d3829bc20c69eabc83032716604b133d2f19cf9))
* **gcp:** add the GKE Autopilot provider ([412d62a](https://github.com/magelift/magelift/commit/412d62a394ce0fcbce3aee22ab27bbca39d0c94c))
* require PHP 8.3 ([ad6cfea](https://github.com/magelift/magelift/commit/ad6cfeaadd40bef1ee05a3eefa915459a9c2e461))

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
