# ADR 0006: Separate GitHub CI and state recovery roles

Status: accepted

## Context

Pulumi and ECS deployment jobs need AWS permissions for the managed resource graph.
State recovery needs only the Pulumi state bucket, its KMS key, and bootstrap metadata.
Using one GitHub OIDC role for both would either block deployments or grant recovery
jobs unnecessary infrastructure permissions.

## Decision

`magelift bootstrap` creates three repository-scoped identities per environment:

- a state role with only state recovery permissions;
- a CI role with explicit actions for the certified v1 AWS resource graph.
- a build role with read-only access to MageLift-scoped build secrets.

The CI role is trusted only for the matching GitHub environment subject. Generated
preview jobs use the protected `preview` environment, and staging and production use
their matching GitHub environments. IAM role and policy mutation is limited to
MageLift-owned names, and ECS `PassRole` is limited to MageLift roles.

## Consequences

Bootstrap output contains all three ARNs. Generated workflows use
`identity.buildRoleArn` for the image build job and `identity.ciRoleArn` for their
environment role variables. The state role remains available for audited recovery
procedures and cannot mutate application infrastructure.

Floci cannot certify IAM or GitHub OIDC behavior. Pulumi mocks and the scheduled AWS
matrix must verify the final policy and trust behavior.

## Alternatives

One broad role was rejected because it violates least privilege. Requiring users to
create the CI role manually was rejected because it breaks the supported bootstrap
path.
