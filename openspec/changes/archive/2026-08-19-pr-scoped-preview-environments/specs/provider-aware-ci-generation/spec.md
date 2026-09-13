## Purpose

Defines provider-aware GitHub Actions generation so MageLift's automation
workflow uses the selected provider's authentication and deployment contract
instead of assuming AWS ECS.

## ADDED Requirements

### Requirement: Generated CI matches the resolved provider target

The system MUST resolve each environment's provider and runtime before
generating a workflow. A generated workflow MUST use only actions, variables,
credentials, and deployment commands supported by that target. The generator
MUST reject a target with an actionable capability error when its CI path is
not implemented, rather than emitting a workflow for another provider.

#### Scenario: A GCP target generates CI

- **WHEN** a project resolves its preview and staging environments to a
  supported GCP runtime
- **THEN** the generated workflow uses the GCP authentication and deployment
  contract and contains no AWS credential action or AWS-only variable

#### Scenario: An unsupported provider CI path is requested

- **WHEN** a project resolves an environment to a provider/runtime without a
  registered CI generator
- **THEN** `ci generate` fails before writing a workflow and names the target
  and the missing capability

### Requirement: GCP CI uses short-lived federation credentials

The GCP workflow MUST request only the GitHub token permissions needed for
Workload Identity Federation and MUST NOT require a long-lived service-account
key in repository secrets. Project, workload identity provider, service
account, region, and state-backend references MUST be supplied through the
configured environment or repository variables and MUST be validated before
provider mutation.

#### Scenario: GCP preview CI authenticates

- **WHEN** a labeled pull request runs the generated GCP preview job
- **THEN** the job requests an OIDC token, authenticates through the configured
  federation provider, validates the target project and backend, and runs the
  preview command without printing a private key

### Requirement: Generated pull request jobs use the preview identity contract

The generated workflow MUST pass the pull request number, commit digest, and
derived preview identity to preview apply and close cleanup. Preview jobs MUST
use a concurrency group scoped to the repository and pull request so an apply
and a close cleanup cannot mutate the same preview stack concurrently. Reruns
of one pull request MUST remain idempotent.

#### Scenario: Two labeled pull requests run at once

- **WHEN** pull requests 41 and 42 start preview jobs in the same repository
- **THEN** each job uses its own concurrency group and derived preview
  identity, while unrelated pull requests can run concurrently

#### Scenario: A close job follows a redeploy

- **WHEN** a pull request is redeployed and its close event is delivered
- **THEN** the close job passes the current identity and generation checks to
  cleanup and does not destroy a preview belonging to another pull request

### Requirement: Workflow validation is deterministic

`ci validate` MUST render the same provider-aware workflow as `ci generate`
for the same configuration and release version. Generated workflows MUST
remain inspectable by repository-local YAML and GitHub Actions validation
tests, including the GCP authentication, preview identity, concurrency, and
unsupported-provider paths.

#### Scenario: Generated GCP workflow is validated

- **WHEN** `ci validate` runs after `ci generate` without configuration
  changes
- **THEN** validation succeeds with the same workflow digest

#### Scenario: Provider configuration changes after generation

- **WHEN** the configured provider changes from GCP to an unsupported runtime
- **THEN** `ci validate` fails rather than accepting an AWS-shaped or stale
  workflow
