# Tests

Run `make verify-community` from the repository root. It exercises the CLI,
SDK, and provider modules, checks race-sensitive paths, and runs the PHP build
package and shell tests. Install the build dependencies with
`composer install --working-dir=build` first.

| Check | What it exercises |
| --- | --- |
| `make test-fast` | Go tests for the CLI, SDK, and each provider, including Pulumi mocks |
| `make local-gates` | The module tests plus AWS, GCP, and Azure API calls through local Floci containers |
| `make ci-act-project` | A generated shop workflow against a local container registry |
| `make magento-dev-smoke` | Local Magento install, HTTP and GraphQL checks, and repeat startup |
| `make script-test` | Shell regressions in [`tests/scripts/`](scripts/) |
| `go test -tags synthetic ./tests/synthetic/` | Config validation, PaaS import, and provider dispatch with invented shop fixtures |
| `make image-test` | nginx/PHP-FPM recipe, health endpoint, and private media rules |
| `make build-e2e-test` | BuildKit, Composer and the finished image from a dependency-free fixture |

The provider Compose files and Floci runners live under `providers/<name>/`.
To check SQL restore and media permissions on local Kubernetes, run
`MAGELIFT_RUN_KIND_RESTORE=1 bash tests/scripts/kind_migration_restore_test.sh`.
Fixtures use synthetic project identities and data. Include the test method and
the precise result when reporting cloud behavior. [Provider support](../docs/capability-matrix.md)
