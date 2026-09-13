## Context

The current generator renders one AWS ECS workflow and passes a configured
preview environment name to every pull request. The configuration layer and
provider registry already resolve provider and runtime targets, but workflow
rendering does not use that information. Existing named environments remain
the right model for staging, UAT, and production.

See the proposal for the product motivation and the two capability specs for
the observable contract.

## Goals / Non-Goals

**Goals:**

- Give every pull request a stable preview identity that is safe across
  concurrent runs and branch renames.
- Keep apply, redeploy, expiration, and close cleanup bound to the same
  ownership and generation record.
- Render provider-aware workflows with GCP federation as the first new
  provider path and preserve a clean extension point for OVHcloud and
  Scaleway.
- Keep provider SDK types and credential formats out of the portable preview
  identity model.
- Make the generated workflow deterministic and testable without creating
  cloud resources.

**Non-Goals:**

- Replacing manually named environments or changing the semantics of staging,
  UAT, or production.
- Adding OVHcloud or Scaleway GitHub authentication in this change.
- Providing a budget enforcement system. Cost reporting remains a separate
  capability.
- Proving provider runtime health or preview application readiness in the
  workflow generator. Those checks remain provider and certification work.

## Decisions

### Use pull request number as the stable logical identity

The canonical identity is the repository slug plus pull request number. The
branch name is stored as current metadata only. Pull request numbers are
stable within a repository, so branch renames do not create a second preview.
The repository component prevents collisions when projects share a Pulumi
backend. A short canonical digest is used where provider or environment name
limits prevent the full repository slug from fitting.

Using the branch slug as the primary identity was rejected because branch
renames would orphan resources and branch names can exceed provider naming
limits. Using the commit SHA was rejected because each commit would create a
new preview rather than update one pull request environment.

### Keep identity separate from provider configuration

The identity value contains repository, pull request, branch metadata, commit
digest, generation, environment name, stack key, owner marker, domain, and
expiration. Provider-specific account, project, region, credentials, and
runtime fields stay in the resolved configuration. This keeps the identity
contract usable by community providers and avoids copying provider SDK maps
into Pulumi state or CI output.

### Store ownership and generation at the lifecycle boundary

Preview apply and cleanup must resolve the identity before any provider
mutation. The lifecycle passes an immutable owner and generation through the
plan and provider ownership metadata. Cleanup first reads the current record,
checks repository, pull request, owner, and generation, then performs the
provider destroy. A stale close or sweep returns a retryable conflict before
mutation.

The current environment name and commit digest are retained in the record so
operators can diagnose a preview without reading provider-specific resources.

### Register workflow generators by provider and runtime

CI rendering uses a small provider-owned generator registry selected from the
already resolved target. The portable renderer owns event wiring, identity
inputs, concurrency, release pinning, and validation. The provider generator
owns authentication steps, required variables, and the deploy command. A
missing generator is an explicit capability error.

This is preferred over adding provider conditionals to one large template. It
keeps AWS compatibility while allowing GCP, OVHcloud, Scaleway, and community
providers to add generators without importing one another's SDKs.

### Queue preview mutations per pull request

Apply and close jobs use one concurrency group per repository and pull request
with cancellation disabled. This serializes operations that mutate one
preview, while unrelated pull requests remain concurrent. Generation checks
remain necessary because queued events can carry stale payloads.

### Use GCP Workload Identity Federation

The GCP generator requests GitHub's OIDC token permission and uses the
configured workload identity provider and service account. It never writes a
service-account key into repository secrets or workflow files. The workflow
validates required project, region, Pulumi backend, and federation references
before invoking MageLift.

## Risks / Trade-offs

- [Risk] A shared Pulumi backend may contain legacy fixed preview stacks.
  [Mitigation] The new identity uses a separate stack key namespace and the
  generator does not destroy a legacy stack unless its owner record matches.
- [Risk] A close event can be queued behind a redeploy and contain an older
  commit. [Mitigation] Disable cancellation and enforce generation checks
  before destroy; report a retryable conflict when the record is newer.
- [Risk] Provider naming limits can make derived names collide.
  [Mitigation] Canonicalize names, include the repository digest, validate
  uniqueness before mutation, and test collision cases.
- [Risk] A generated workflow can drift from the provider's supported auth
  contract. [Mitigation] Keep generator registration explicit, validate all
  required variables, pin actions by repository policy, and run YAML and
  GitHub workflow shape checks in CI.
- [Risk] Moving the AWS generator from one fixed preview to PR-scoped previews
  changes existing automation after regeneration. [Mitigation] Keep manual
  fixed environments compatible, make `ci validate` fail with a migration
  instruction, and document the generated workflow change.

## Migration Plan

1. Add the identity and lifecycle contract without changing manually named
   environments.
2. Update the AWS generator to emit the identity contract and add the GCP
   generator with federation.
3. Regenerate `.github/workflows/magelift.yml` in example and fixture projects.
4. Run local YAML, actionlint, and workflow-shape tests. Use `act` only for
   jobs whose provider authentication can be stubbed safely.
5. If a generated workflow fails validation, keep the previously committed
   workflow and report the exact provider capability or migration error.

Rollback is a repository change: restore the previous generated workflow and
continue using manually named environments. Existing Pulumi stacks are not
deleted by rollback.

## Open Questions

- Whether the final GCP workflow should use a provider-specific deploy role
  per environment or one role constrained by environment claims can be
  decided during implementation without changing the identity contract.
