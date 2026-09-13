## MODIFIED Requirements

### Requirement: Dual-mode module loading

MageLift MUST load first-party and community modules through the same versioned public contract, including web-runtime plugins and observability plugins. Tests, Floci suites, and acceptance harnesses MUST be able to load a module in-process. The published CLI MUST load a community module as a signed subprocess that speaks that contract over HashiCorp go-plugin (gRPC). First-party adapters MAY compile into the default binary but MUST still register on that host. MageLift MUST NOT use Go `plugin.Open` or execute unsigned files from the working directory. Web-runtime descriptors MUST validate API version, plugin ID, Magento range, and image contract, and MUST NOT require stack core output keys. `Plan` MUST use the caller context.

#### Scenario: Tests stay in-process

- **WHEN** `go test` or `make floci-test-aws` exercises a first-party module
- **THEN** the module runs in-process through the public SDK contract and does not require a downloaded provider binary

#### Scenario: Published CLI uses a subprocess

- **WHEN** a released `magelift` binary plans or deploys a registered provider whose artifact is installed and verified
- **THEN** provider code runs in a HashiCorp go-plugin subprocess whose identity matches the lockfile digest and advertised SDK API version

#### Scenario: Tests stay in-process for web runtimes

- **WHEN** `go test` exercises the first-party nginx-fpm web-runtime plugin
- **THEN** the plugin runs in-process through the public SDK contract

#### Scenario: Community Apache plugin is unsigned

- **WHEN** a remote Apache web-runtime artifact lacks a configured trust identity or digest
- **THEN** MageLift refuses to install or execute it
