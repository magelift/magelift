# Contributing

Thanks for helping. MageLift is pre-alpha; open an issue before a large change.

## Prerequisites

- Go and toolchain from [`go.mod`](go.mod) (CI uses `go-version-file`)
- Docker for Floci, images, and local Compose
- PHP 8.2+ and Composer when you touch the `build/` Magento package
- AWS or GCP credentials only if you run real-cloud acceptance

Layout:

- `cmd/` — CLI and generators
- `internal/` — CLI, config, deploy, automation, `platform`, `cloud/<provider>`
- `sdk/v1/` — portable Target / Capability / Hook types
- `build/` — Composer Magento package (not the Go build tree)
- `images/` — PHP runtime and FrankenPHP
- `schema/` — generated JSON Schema for `magelift.yaml`
- `docs/` — MkDocs; ADRs in `docs/adr/`
- `tests/` — Floci and fixtures ([tests/README.md](tests/README.md))

## Principles

- Keep the normal user path YAML-only.
- Prefer per-provider adapters over shared cloud graphs (ADR 0002, 0008).
- Keep config and deploy behavior deterministic.
- Do not copy private or employer-owned source, docs, IDs, or secrets.
- Record design inputs in `docs/provenance.md`; keep third-party notices.
- Never commit credentials, customer data, build artifacts, or Pulumi state.

## Changes

1. Branch from `main`.
2. Add tests and docs with behavior changes.
3. Run `make verify` plus any targeted checks under Testing.
4. Conventional Commit subject, e.g. `feat(config): explain provenance`.
5. Open a PR with the template (risk, compatibility, how you verified).

Signed releases need attributable commits. Maintainers may ask for signed commits
or a DCO before a public release. Contributions are Apache-2.0 unless stated
otherwise.

## Testing

| Goal | Command |
| --- | --- |
| Default local gate | `make verify` |
| Account-free AWS paths | `make floci-test` |
| Real AWS (destroy on exit) | `make aws-acceptance-local` — read `docs/aws-acceptance.md` |
| Real GCP experimental | `make gcp-acceptance-local` — read `docs/gcp-acceptance.md` |

More detail: [tests/README.md](tests/README.md).

## Adding a provider

CLI planning and Pulumi programs go through `platform.ModuleRegistry`
(`RegisterModule`). `internal/infra.Registry` alone does not wire deploy.
Checklist: [docs/adding-a-provider.md](docs/adding-a-provider.md). Rules: ADR 0004,
0007, 0008.

## Architecture decisions

Hard-to-reverse boundary changes need an ADR under `docs/adr/`. Amend an existing
ADR when the decision evolves.

## Compatibility and security

Do not quietly weaken compatibility, protection, validation, or secret handling.
Report vulnerabilities via `SECURITY.md`, not a public issue.

Follow `CODE_OF_CONDUCT.md`.
