# Tests

Verification layers vs Make targets and CI.

| Layer | Location | How to run | Cloud account? |
| --- | --- | --- | --- |
| Go unit / package | `internal/**`, `sdk/v1/**` | `make test` | No |
| Floci (AWS emulator) | `tests/floci/` (`//go:build floci`) | `make floci-test` | No (Docker) |
| PHP Composer package | `build/tests/` | `make php-test` | No |
| Runtime / Varnish / build e2e | `scripts/*.sh` | `make image-test`, `varnish-test`, `build-e2e-test` | No (Docker) |
| Real AWS acceptance | `scripts/aws-acceptance-local.sh` | `make aws-acceptance-local` | Yes — destroy on exit |
| Real GCP acceptance (experimental) | `scripts/gcp-acceptance-local.sh` | `make gcp-acceptance-local` | Yes — destroy on exit |

## Daily path

1. `make verify` — format, schema/docs drift, lint, race tests, licenses, PHP, MkDocs, actionlint.
2. `make floci-test` when you change AWS bootstrap, locks, secrets, logs, media restore, or ECS candidates.

Skip real-cloud acceptance unless you own the account and have read:

- [AWS acceptance](../docs/aws-acceptance.md)
- [GCP acceptance](../docs/gcp-acceptance.md)

Those scripts destroy on EXIT. Killing them mid-run can leave orphans and Pulumi
locks.

## Fixtures

E2E fixtures live under `tests/fixtures/`. No credentials or customer data.
