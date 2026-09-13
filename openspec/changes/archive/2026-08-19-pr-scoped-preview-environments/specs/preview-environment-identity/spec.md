## Purpose

Defines a stable, ownership-scoped identity for disposable pull request
environments so concurrent preview deployments cannot share or destroy one
another's infrastructure.

## ADDED Requirements

### Requirement: Pull request previews have deterministic identities

The system MUST derive a preview identity from the repository or project
identity and pull request number. The identity MUST produce a valid MageLift
environment name, a unique Pulumi stack key, and an ownership marker. The
source branch name MAY be retained for display and diagnostics, but it MUST
NOT be the only identity component. Fixed environments such as staging, UAT,
and production MUST keep their configured names and ownership scopes.

#### Scenario: Two pull requests deploy previews concurrently

- **WHEN** pull requests 41 and 42 deploy from the same repository
- **THEN** MageLift resolves different environment names, stack keys, and
  ownership markers and neither deployment targets the other's resources

### Requirement: Repeated preview deployment is idempotent

The system MUST reuse the same preview identity when the same pull request is
deployed again. A new commit MUST update that identity with the commit digest
and deployment generation without creating a second preview environment. A
branch rename MUST NOT orphan the preview or change its pull request identity.

#### Scenario: A pull request is redeployed after a new commit

- **WHEN** pull request 41 is deployed at commit A and then at commit B
- **THEN** both runs resolve the same environment and stack identity, the
  current generation records commit B, and no duplicate preview owner exists

#### Scenario: A pull request branch is renamed

- **WHEN** the source branch for pull request 41 changes from `feature/cart`
  to `feature/cart-v2`
- **THEN** the preview identity remains the identity for pull request 41 and
  the new branch name is stored only as current diagnostic metadata

### Requirement: Preview cleanup is ownership and generation safe

Close-event cleanup MUST target only the preview identity for the referenced
pull request and MUST verify the expected ownership marker before mutation. If
the recorded generation is newer than the close event's generation, cleanup
MUST refuse the destructive operation and return a retryable, actionable
diagnostic. Expiration sweeps MUST apply the same ownership check and MUST NOT
remove fixed environments or previews owned by another pull request.

#### Scenario: A close event races with a newer preview deployment

- **WHEN** cleanup for pull request 41 observes a deployment generation newer
  than the generation carried by the close event
- **THEN** MageLift leaves the newer deployment untouched and reports the
  identity and generation that require a fresh cleanup decision

#### Scenario: An expired preview has a foreign owner

- **WHEN** a sweep finds an expired environment whose ownership marker does not
  match the derived pull request identity
- **THEN** MageLift reports the ownership conflict and does not destroy the
  environment

### Requirement: Preview identity metadata is safe to expose

Preview identity metadata MUST contain only repository, pull request, branch,
commit, generation, ownership, domain, and expiration values. It MUST NOT
contain cloud credentials, secret values, access tokens, or provider SDK
argument maps in configuration, Pulumi state diagnostics, workflow output, or
evidence.

#### Scenario: A preview identity is rendered in CI output

- **WHEN** the workflow prints the resolved preview identity
- **THEN** the output contains the environment and ownership identifiers but
  no credential or secret value
