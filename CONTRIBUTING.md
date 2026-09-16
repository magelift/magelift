# Contributing

Thanks for helping. Open an issue before a large change.

## Agents

Root [`AGENTS.md`](AGENTS.md) is the always-on agent guide. Contributor skills
live under [`contrib/skills/`](contrib/skills/). User skills live under
[`agents/`](agents/README.md) and are the only skills embedded in release
binaries. Symlink contributor skills into your local `.cursor/skills` or
`.claude/skills` when useful; those dirs stay gitignored. Do not commit
personal agent caches or third-party skill packs.

## 30-minute first PR

Pick a `good first issue`, or:

1. Fix a typo / dead link in `docs/` or `README.md`.
2. Add a unit test beside an existing `_test.go` that already covers a nearby case.
3. Extend `examples/custom-cli` comments if a provider-author step confused you.

Then:

```sh
# After: composer install --working-dir=build  (once)
make verify   # or at least: make docs && GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/<pkg>/ -count=1
```

Open a PR with: what changed, how you verified, risk (usually “docs/test only”).
Maintainers merge Dependabot when CI is green and the bump is patch/minor in an
already-pinned ecosystem group; major bumps need a human review note.

## Prerequisites

For a full green `make verify` (the default local gate):

- **Go** toolchain matching [`go.mod`](go.mod) (CI uses `go-version-file`)
- **PHP 8.2+** and **Composer**, required because `make verify` runs `php-test`
  on the `build/` Magento package. After clone, once: `composer install --working-dir=build`
- **MkDocs**, required because `make verify` runs `docs` (strict link/nav build)

Pulled automatically via pinned `go run` from Makefile targets (no separate
global install needed for these):

- golangci-lint (`make lint`)
- go-licenses (`make license-check`)
- actionlint (`make workflow-check`)

Optional / situational:

- **Docker**, only for Floci, image builds, and local Compose (`make floci-test-aws`,
  `make floci-gcp-test`, `make local-gates`, `magelift local`, image targets). Not required for default `make verify`.
- **AWS or GCP credentials**, only for real-cloud acceptance targets
  (`make aws-acceptance-local`, `make gcp-acceptance-local`).

Layout:

- `cmd/`: CLI and generators
- `internal/`: CLI, config, deploy, automation, `platform`, `cloud/<provider>`
- `sdk/`: portable Target / Capability / Hook types
- `build/`: Composer Magento package (not the Go build tree)
- `images/`: PHP-FPM + nginx runtime (FrankenPHP classic remains in-tree for provenance; schema rejects it as `application.webRuntime`)
- `schema/`: generated JSON Schema for `magelift.yaml`
- `docs/`: MkDocs; ADRs in `docs/adr/`; short evidence pack in `docs/evidence/`
- `.agents/knowledge/`: agent OKF bundle (tracked; other `.agents/` caches are not)
- `tests/`: Floci and fixtures ([tests/README.md](tests/README.md))

## Principles

- Keep the normal user path YAML-only.
- Prefer per-provider adapters over shared cloud graphs (ADR 0003, 0004).
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
| Account-free stack (mocks + harness + Floci) | `make local-gates` |
| Pulumi mock graphs | `make pulumi-mock-test` |
| Account-free AWS paths | `make floci-test-aws` |
| Account-free GCP paths | `make floci-gcp-test` |
| Real AWS (destroy on exit; light smoke) | `make aws-acceptance-local`; read `docs/aws-acceptance.md` |
| Real GCP (thorough certified path; destroy on exit) | `make gcp-acceptance-local`; read `docs/gcp-acceptance.md` |

### Local verification vs hosted CI

The contributor gate is local **`make verify`**. Hosted GitHub Actions runs on
the public `magelift/magelift` repo. Path-filtered CI on `main` must stay green
before cutting `v1.0.0-rc.1`. Lint partition timing and policy live in
[docs/lint-policy.md](docs/lint-policy.md). See also
[docs/release-readiness.md](docs/release-readiness.md).

Optional: `make ci-act-go` runs Go CI jobs locally via nektos/act (serial). It is
**not** a substitute for full `make verify` (missing php-test, docs, and other
local targets).
More detail: [tests/README.md](tests/README.md).

## Adding a provider

CLI planning and Pulumi programs go through `platform.ModuleRegistry`
(`RegisterModule`). `internal/infra.Registry` alone does not wire deploy.
Checklist: [docs/adding-a-provider.md](docs/adding-a-provider.md). Rules: ADR 0003,
0004, 0008.

## Architecture decisions

Hard-to-reverse boundary changes need an ADR under `docs/adr/`. Amend an existing
ADR when the decision evolves.

## Compatibility and security

Do not quietly weaken compatibility, protection, validation, or secret handling.
Report vulnerabilities via `SECURITY.md`, not a public issue.

Follow `CODE_OF_CONDUCT.md`.
