# Tests

Use these commands to run the same checks locally.

| Layer | Location | How to run | Cloud account? |
| --- | --- | --- | --- |
| Go unit / package | Root, `sdk/`, and every module under `providers/` | `make test-fast` | No |
| Community alpha | Core, SDK, provider modules, PHP and local contracts | `make verify-community` | No |
| Targeted race | CLI lifecycle, SDK and provider operations | `make test-race` | No |
| Pulumi mock graphs | `internal/cloud/**`, AWS, GCP and Azure provider stacks | `make pulumi-mock-test` | No |
| Offline harness | `tests/acceptance/` | `make acceptance-harness-test` | No |
| Account-free stack | provider module tests + mocks + harness + Floci | `make local-gates` | No (Docker for Floci) |
| Synthetic offline | `tests/synthetic/` (`//go:build synthetic`) | `go test -tags synthetic ./tests/synthetic/ -count=1` | No (refuses credentials) |
| Floci AWS | `providers/aws/floci/` (`//go:build floci`) | `make floci-aws-test` | No (Docker) |
| Floci GCP | `providers/gcp/floci/` (`//go:build floci`) | `make floci-gcp-test` | No (Docker) |
| Floci Azure | `providers/azure/floci/`, Blob and Key Vault SDKs | `make floci-azure-test` | No (Docker) |
| Consumer CI workflow and OCI handoff | `tests/act/`, local registry | `make ci-act-project` | No (Docker and act) |
| Azure database TLS | TLS-only MySQL, valid and invalid CA and hostname with PHP | `make azure-mysql-tls-test` | No (Docker, OpenSSL and a local PHP runtime image) |
| Real Magento locally | Locked, dependency-installed OSS fixture or operator-supplied licensed shop | `MAGELIFT_MAGENTO_SOURCE=/path/to/clean/shop make magento-dev-smoke` | No (Docker) |
| PHP Composer package | `build/tests/` | `make php-test` | No |
| Runtime / Varnish / build e2e | `scripts/*.sh` | `make image-test`, `varnish-test`, `build-e2e-test` | No (Docker) |
| Real AWS acceptance | `scripts/aws-acceptance-local.sh` | `make aws-acceptance-local` | Yes, light smoke, destroy on exit |
| Real GCP acceptance | `providers/gcp/scripts/gcp-acceptance-local.sh` | `make gcp-acceptance-local` | Yes, thorough E2E, destroy on exit |

## Daily path

1. `make verify-community` runs the core, SDK and standalone provider suites. `make test-fast` also covers the embedded OVH, Scaleway and vendor packages. `make verify` checks formatting, generated files, lint, races, licenses, docs and workflows.
2. `make local-gates` when you change providers, Floci contracts, or acceptance harnesses.
3. `make floci-aws-test` alone when you change AWS bootstrap, locks, secrets, logs, media restore, ECS candidates, or IAM OIDC.
4. `make floci-gcp-test` for Cloud Storage, Secret Manager, Pub/Sub, logging or monitoring changes.
5. `make floci-azure-test` for Blob Storage, state locks or Key Vault changes.

Skip real-cloud acceptance unless you own the account and have read:

- [AWS acceptance](../docs/aws-acceptance.md)
- [GCP acceptance](../docs/gcp-acceptance.md)

Those scripts destroy on EXIT. Killing them mid-run can leave orphans and Pulumi
locks.

## Fixtures

E2E fixtures live under `tests/fixtures/`. No credentials or customer data.

The original [Magento shop overlay](fixtures/magento-shop-overlay/README.md)
adds a disabled custom module and a Luma child theme to an owned synthetic shop
for real build acceptance. It is separate from the offline PHP project fixtures;
using the overlay does not prove a running storefront or a provider cell.
