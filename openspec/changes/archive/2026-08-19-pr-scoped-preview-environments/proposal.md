## Why

MageLift currently generates one fixed preview environment for every pull
request. Two concurrent pull requests can therefore share a Pulumi stack, and
closing one pull request can destroy infrastructure still used by another.
The generated workflow also rejects every target that is not AWS ECS, even
though the configuration and provider registry support GCP, OVHcloud, and
Scaleway. Preview identity and CI generation need a provider-neutral contract
before more provider certification is added.

## What Changes

- Add a deterministic preview identity derived from the pull request number,
  with the branch slug as a validated display and diagnostic field.
- Bind the identity to the MageLift environment name, Pulumi stack key,
  ownership marker, domain, commit digest, and expiration policy.
- Make repeated runs for one pull request idempotent and keep two pull
  requests isolated from one another.
- Make pull request close cleanup target the exact pull request identity and
  refuse to destroy a newer deployment that does not match the close event's
  ownership and generation data.
- Keep fixed environments such as staging, UAT, and production unchanged.
- Add provider-owned GitHub Actions workflow generation, starting with GCP
  Workload Identity Federation and the existing GCP deployment path.
- Return an explicit capability error for providers whose CI authentication
  and deployment workflow has not been implemented instead of generating an
  AWS-shaped workflow.
- Add deterministic tests for concurrent pull requests, reruns, branch
  renames, close-after-redeploy, generated GCP workflow shape, and unsupported
  provider diagnostics.

## Capabilities

### New Capabilities

- `preview-environment-identity`: PR-scoped identity, ownership, lifecycle,
  cleanup, and concurrency rules for disposable preview environments.
- `provider-aware-ci-generation`: provider-specific GitHub Actions workflow
  generation with GCP as the first non-AWS implementation.

### Modified Capabilities

## Impact

- `internal/cli/ci.go` and its tests will gain provider-aware workflow
  rendering and PR identity inputs.
- `internal/cli/env.go`, `internal/config`, and the Pulumi stack naming path
  will carry preview ownership and generation metadata.
- GitHub Actions workflows will use the provider's configured authentication
  contract. The GCP path will use Workload Identity Federation and will not
  require a long-lived service-account key.
- Existing fixed environment names and manual `magelift env create` flows
  remain compatible. The change adds an automation path rather than replacing
  those environments.
- No provider API types will be added to the portable identity contract.
