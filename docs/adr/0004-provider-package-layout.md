# ADR 0004: Group provider and build implementations by responsibility

- Status: Accepted
- Date: 2026-07-18

## Context

Provider-specific code will grow as MageLift adds more AWS capabilities and later
supports EKS or another cloud. The PHP Composer package also needs a clear boundary
from the Go build system.

## Decision

Provider implementations live under `internal/cloud/<provider>`. AWS capabilities
use separate packages such as `bootstrap`, `network`, `security`, `ingress`, `runtime`,
`database`, `cache`, `search`, `queue`, `storage`, `observability`, and `stack`.
Provider-neutral orchestration and
contracts stay in `internal/platform` (stack modules and Magento-shaped ports),
`internal/automation`, `internal/deploy`, `internal/infra` (SDK extension index),
`internal/topology`, and `sdk/v1`. Magento-shaped stack registration for the CLI
uses `platform.ModuleRegistry` (ADR 0008); `infra.Registry` alone does not wire
deploy.

The Go build system uses `internal/build/kit`, `internal/build/pipeline`,
`internal/build/plan`, and `internal/build/runner`. The repository-level `build/`
directory remains the Magento Composer package.

Each package validates typed inputs before registering resources. Resource names and
component tokens are stable, and released renames require Pulumi aliases or an
explicit migration.

## Consequences

The directory layout makes provider ownership visible and limits imports between
capabilities. Adding a provider does not require moving shared orchestration code.
The tradeoff is more packages, which is acceptable because each package has one
resource ownership boundary and its own mock graph tests.
