# ADR 0007: Separate GitHub OIDC roles for CI and state recovery

- Status: Accepted
- Date: 2026-08-22

## Context

Pulumi deploy jobs need graph permissions. State recovery needs the state bucket, its KMS key, and bootstrap metadata. One GitHub OIDC role for both is either too weak for deploy or too strong for recovery.

## Decision

`magelift bootstrap` creates repository-scoped identities per environment:

- a state role limited to state recovery
- a CI role with explicit actions for the certified resource graph
- a build role with read-only access to MageLift-scoped build secrets

The CI role is trusted only for the matching GitHub environment subject. Generated preview jobs use the protected `preview` environment. IAM mutation is limited to MageLift-owned names. ECS `PassRole` is limited to MageLift roles.

## Consequences

Bootstrap output contains all three ARNs. Generated workflows use `identity.buildRoleArn` for image build and `identity.ciRoleArn` for environment roles. Floci cannot certify IAM or GitHub OIDC; Pulumi mocks and sparse live AWS runs must.

## Alternatives considered

- One broad role: rejected (least privilege).
- Operators create the CI role by hand: rejected (breaks bootstrap).

## Provenance

Original. AWS IAM and GitHub OIDC public docs.

## GCP community alpha boundary

GCP uses separate build and deployment service accounts in one environment-scoped
workload identity pool. The provider accepts only the named repository's
`.github/workflows/magelift.yml` on `refs/heads/main`, for `push` or
`workflow_dispatch`, with a GitHub environment of `build` or the selected shop
environment. Service-account impersonation is bound to the corresponding
`attribute.environment` principal set. The shop environment cannot be named
`build`. This boundary uses `workflow_ref`; `job_workflow_ref` is available only
for reusable workflows.

Bootstrap refuses existing pools, providers and service accounts whose exact
ownership description differs. It also refuses a changed issuer, attribute
mapping or condition. IAM updates preserve policy version, conditions, audit
configuration and etag; a conflict triggers a fresh read before one retry.
Short existing resource IDs are retained; long IDs receive a deterministic hash.

This establishes the provider's identity construction and reconciliation
contract. It does not grant registry or deployment permissions by itself. The
command still needs the account-preparation integration. Local API fixtures do
not prove Google CEL evaluation or a GitHub-to-GCP token exchange; both remain
acceptance checks.
