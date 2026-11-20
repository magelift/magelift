# ADR 0007. Use separate identities for build, deploy and recovery

Accepted on 2026-08-22.

A build runner needs to push an image and read the shop's build credentials.
A deploy runner manages a Magento environment. State recovery touches the
state bucket and its encryption key. Separate identities limit which job
can use each permission.

## AWS

Bootstrap with GitHub options returns separate build, deployment and state
role ARNs. Generated workflows use `identity.buildRoleArn` and
`identity.ciRoleArn` for their respective jobs. Trust is scoped to the repository and the matching GitHub
environment. The build role can read MageLift-scoped build secrets. The
state role is reserved for recovery. The deployment policy limits role
management and ECS role passing to MageLift roles.

## Google Cloud

Bootstrap binds separate build and deployment service accounts to GitHub
environments. The federation provider checks the repository and owner IDs,
the workflow path and `refs/heads/main`. Stable environments accept push and
manual dispatch. Preview also accepts the preview jobs on
`pull_request_target` and its scheduled cleanup.

Bootstrap checks identity ownership before reuse and uses etags when changing
IAM. The build identity receives registry write access. The deploy identity
needs state and project-level service permissions, plus impersonation of the
node account. Bootstrap returns those scopes for review. Use
`--apply-ci-permissions` to grant them and `--configure-github` to write the
repository variables. [Cloud setup](../bootstrap.md) · [GKE node identity](0016-gcp-owned-node-identity.md)

## Azure

Set up an Entra federated identity for the GitHub deployment environment.
Grant it access to the resources and Blob state account the workflow uses.
[Azure setup](../bootstrap.md#azure-aks)

## Verification

Local tests inspect role permissions, trust conditions and IAM updates.
Verify token exchange and permissions with the shop's staging identities.
