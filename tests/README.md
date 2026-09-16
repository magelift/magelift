# Tests

Verification layers vs Make targets and CI.

| Layer | Location | How to run | Cloud account? |
| --- | --- | --- | --- |
| Go unit / package | `internal/**`, `sdk/**` | `make test` + `make sdk-test` | No |
| Pulumi mock graphs | `internal/cloud/**` | `make pulumi-mock-test` | No |
| Offline harness | `tests/acceptance/` | `make acceptance-harness-test` | No |
| Account-free stack | mocks + harness + Floci | `make local-gates` | No (Docker for Floci) |
| Floci (AWS emulator) | `tests/floci/` (`//go:build floci`) | `make floci-test-aws` | No (Docker) |
| floci-gcp | `tests/floci-gcp/` (`//go:build floci_gcp`) | `make floci-gcp-test` | No (Docker) |
| PHP Composer package | `build/tests/` | `make php-test` | No |
| Runtime / Varnish / build e2e | `scripts/*.sh` | `make image-test`, `varnish-test`, `build-e2e-test` | No (Docker) |
| Real AWS acceptance | `scripts/aws-acceptance-local.sh` | `make aws-acceptance-local` | Yes, light smoke, destroy on exit |
| Real GCP acceptance | `scripts/gcp-acceptance-local.sh` | `make gcp-acceptance-local` | Yes, thorough E2E, destroy on exit |

## Daily path

1. `make verify`: format, schema/docs drift, lint, race tests, licenses, PHP, MkDocs, actionlint.
2. `make local-gates` when you change providers, Floci contracts, or acceptance harnesses.
3. `make floci-test-aws` alone when you change AWS bootstrap, locks, secrets, logs, media restore, ECS candidates, or IAM OIDC.
4. `make floci-gcp-test` alone when you change GCP object, secret, Pub/Sub, logging, or monitoring SDK paths that floci-gcp covers.

Skip real-cloud acceptance unless you own the account and have read:

- [AWS acceptance](../docs/aws-acceptance.md)
- [GCP acceptance](../docs/gcp-acceptance.md)

Those scripts destroy on EXIT. Killing them mid-run can leave orphans and Pulumi
locks.

## Fixtures

E2E fixtures live under `tests/fixtures/`. No credentials or customer data.
