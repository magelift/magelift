# Bootstrap

`magelift bootstrap` prepares the selected cloud account so Magento can deploy.
Laptop users omit GitHub flags. GitHub Actions needs `--github-owner` and
`--github-repo` so Magelift can create the CI identity.

You do not set a state-backend URL. Magelift derives it from `magelift.yaml`
after bootstrap.

For an existing GCP state bucket, bootstrap checks its four MageLift ownership
labels and region before changing versioning or retention. A mismatch stops
without adopting the bucket. Select a fresh project/environment name or have
the bucket owner review its recorded ownership; do not relabel another bucket
to bypass the check. This guard checks labels and region, not the cloud project
number.

## Laptop

Log in with `aws` or `gcloud`, then:

```sh
magelift bootstrap --env preview
```

AWS also needs an existing log bucket:

```sh
magelift bootstrap --env preview --access-log-bucket existing-log-bucket
```

Then `magelift deploy --env preview --yes`. Destroy with
`magelift destroy --env preview --yes`.

## AWS details

`magelift bootstrap` verifies the active caller account before changing anything, then creates the Pulumi S3
state bucket, its KMS key, a recovery-only state identity, and (when GitHub flags are set) a GitHub Actions
infrastructure identity. It also creates a separate build identity that can read
only MageLift-scoped Composer and build secrets.

The access log bucket must already exist. MageLift does not create or delete that
shared logging bucket. State backups under `backups/` expire after 90 days, and
incomplete multipart uploads are removed after seven days. Current Pulumi state
objects are retained by versioning and are not covered by the backup expiry rule.

```sh
magelift bootstrap --env staging \
  --access-log-bucket existing-log-bucket \
  --github-owner magelift \
  --github-repo magelift
```

The command accepts only an AWS account selected by the environment. If the active
credentials belong to another account, it stops before any resource mutation.
State and identity details are printed in the selected output format. Secret values
are never included. Use `identity.buildRoleArn` for `MAGELIFT_BUILD_ROLE_ARN` and
`identity.ciRoleArn` for the environment role variables consumed by generated
deployment workflows. Keep `identity.stateRoleArn` for audited state recovery only.

Configure the GitHub `preview`, `staging`, and `production` environments with the
required approvals and use their environment variables for the corresponding CI role
ARNs. The role trust policy accepts environment subjects, not arbitrary pull-request
branches. Re-running bootstrap reconciles the managed policy versions before it
updates role policies.

## Generated CI and pull-request previews

The minimal starter produces build and test-deploy jobs. On GCP they use
separate GitHub environments named `build` and `test`. The workflow moves
federated credentials into the runner temporary directory before building;
the Google auth post hook removes them after the job. Customized workflows
must also keep those credentials outside the shop source. The build refuses
known root `gha-creds-*.json` files.


Generate the workflow from the same configuration that will be deployed:

```sh
magelift ci generate
magelift ci validate
```

Generation uses the installed MageLift release version. Development binaries
need an explicit --magelift-version naming an available signed release.
Generation preserves an existing customized workflow and writes a candidate
beside it. Review the reported diff, then rerun the original generation command
with `--replace` if you want to adopt the template. Retain your `--path` and
version flags. Validation checks configuration, presence and YAML syntax;
`--check-template` additionally requires exact template content. This is not a
validator for arbitrary GitHub Actions behavior.


The generator selects the registered provider/runtime adapter. AWS ECS Fargate uses
the role variables created by `magelift bootstrap --github-owner --github-repo`.
GCP GKE uses GitHub federation variables Magelift prints after that same bootstrap:
`MAGELIFT_GCP_PROJECT_ID`, `MAGELIFT_GCP_REGION`,
`MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER`, and
the separate `MAGELIFT_GCP_BUILD_SERVICE_ACCOUNT`,
`MAGELIFT_GCP_TEST_SERVICE_ACCOUNT`, `MAGELIFT_GCP_STAGING_SERVICE_ACCOUNT`, `MAGELIFT_GCP_PRODUCTION_SERVICE_ACCOUNT`
and `MAGELIFT_GCP_PREVIEW_SERVICE_ACCOUNT` variables for the jobs you enable.
Bootstrap creates separate build and selected-environment federation identities.
It resolves the repository and owner numeric IDs through the current GitHub
login before changing cloud resources. Use GH_TOKEN, GITHUB_TOKEN, or gh auth
login. Bootstrap returns the registry and deployment permission plan separately.
Review it before rerunning with --apply-ci-permissions. Creating federation
identities alone does not grant those permissions.
The runtime's GKE image-pull identity is another role and needs repository read
permissions independently of the Pod's application identity.

Magelift derives stack state from `magelift.yaml` after bootstrap; do not set a
state-backend URL. Set `MAGELIFT_RELEASE_IMAGE` to your application destination
and `MAGELIFT_REGISTRY` to its exact registry host. Artifact Registry uses the
GCP federation credential helper. User GHCR uses GITHUB_TOKEN; other registries
require `MAGELIFT_REGISTRY_USERNAME` and secret `MAGELIFT_REGISTRY_PASSWORD` or an
explicitly prepared credential flow. The workflow never sends GITHUB_TOKEN to
an arbitrary registry. Native ECR workflow authentication remains a later AWS
acceptance check.

The source-build path needs no builder/runtime image variables. Configure
`build.composer.credentials: env://COMPOSER_AUTH` to consume the optional
`MAGELIFT_COMPOSER_AUTH` GitHub secret. See [builds](builds.md) for the cache and
recipe inspection options.

The GCP workflow requests the GitHub OIDC token and exchanges it for short-lived
credentials. It never reads or writes a service-account key. Before a provider call,
it checks the configured project, region, federation references, state backend, and
selected environment.

Label a pull request `magelift-preview` to enable its preview job. MageLift derives
one environment and Pulumi stack from the repository and pull-request number, so a
branch rename or a new commit updates the same preview. The apply and close jobs use
the same repository/pull-request concurrency group and carry the deployment
generation. A close event that is older than the recorded deployment is refused
before mutation and can be retried with a fresh event. The scheduled sweep uses the
same ownership check and does not touch staging, UAT, production, or another
repository's preview.

## Account-free contract tests

Run `make floci-test-aws` when Docker is available. The pinned [Floci](https://github.com/floci-io/floci)
1.7.0 image exercises
KMS key reconciliation, S3 state setup and versioned-media restore, conditional lock
acquisition, ownership diagnostics, lock release, ECS candidate registration, Secrets
Manager version updates, and CloudWatch Logs ingestion without an AWS account.
Run `make floci-gcp-test` for GCS, Secret Manager, Pub/Sub, Logging, and Monitoring contracts against pinned floci-gcp 0.7.0.

Floci is an emulator, not a cloud certification environment. Packed live sessions
are documented in [certification sessions](certification-sessions.md). Floci AWS
1.7.0 implements IAM Create/Get/Tag OpenID Connect Provider (`make floci-test-aws`
`TestIAMOpenIDConnectProviderAgainstFloci`). That is an API contract, not GitHub
Actions token exchange. Do not weaken the production identity contract. Managed-service
behavior is not proof of AWS behavior. Identity bootstrap against a real account
and GitHub OIDC from CI still need a sparse local real-AWS acceptance pass when
credits allow; see [Local AWS acceptance](aws-acceptance.md).

For local AWS-compatible testing, set `MAGELIFT_AWS_ENDPOINT_URL` to the emulator
endpoint, for example `http://127.0.0.1:4566`. The endpoint must use HTTP or HTTPS
and resolve to loopback with no credentials or path. Bootstrap S3/KMS clients, the
state archive, and Secrets Manager/SSM clients honor it; S3 clients use path-style
requests. For floci-gcp, set `MAGELIFT_GCP_ENDPOINT_URL` to `http://127.0.0.1:4588`
and `STORAGE_EMULATOR_HOST` to the host:port. Production runs leave the variables unset.

The build job retains the existing manifest, checksum and build result for 14
days in the user's repository. Staging verifies that handoff before deployment.
Production dispatch requires `release_digest` and `artifact_run_id` from a
successful run of the same workflow in the same repository; it does not build.
The guided GCP setup derives `MAGELIFT_IMAGE_SIGNING_IDENTITY` and
`MAGELIFT_IMAGE_SIGNING_ISSUER` from the repository workflow, independently of
the deployment login. Advanced workflows can configure their producer policy
explicitly.
Configure required reviewers on protected GitHub environments. Fork PRs do not
receive build, preview or deployment privileges; labelled preview jobs also
require a branch in the same repository.

The generated workflow is tested locally as code. A local build or fixture test
is not evidence of an executed GitHub/GCP workflow; record that acceptance run
separately before advertising the full alpha path.

The GCP provider's identity reconciler distinguishes build and deployment
service accounts. Its reference trust accepts the named repository's
`.github/workflows/magelift.yml` on `main`, for push or manual dispatch, and
separates impersonation through the `build` and selected shop environments.
An existing resource with different ownership or federation settings is refused;
bootstrap does not silently adopt it. IAM reconciliation preserves unrelated
bindings and conditions. This library behavior is covered by local fixtures;
a real GitHub token exchange remains an acceptance check.

An untagged `MAGELIFT_RELEASE_IMAGE` destination receives the source commit SHA
as its tag in CI. An explicitly tagged destination is preserved. Build results
stay in the runner temporary directory so the source checkout remains clean.
The build job installs a pinned Docker Engine before Buildx instead of using
the runner image's bundled version.

## GCP registry and state key

GCP bootstrap enables missing reference APIs, ensures the state bucket,
initializes its state passphrase in Secret Manager, and creates the user's
Docker Artifact Registry repository. It also creates an owned GKE node service
account, grants it reader on that repository and the project-level
`roles/container.defaultNodeServiceAccount` role. This runtime preparation
runs even without GitHub options; it needs project IAM policy management
permission. It returns the image destination, node identity and secret version
reference. The caller needs permission to create these
resources. If Service Usage itself is unavailable, the error names that
prerequisite. GitHub CI grants require the explicit --apply-ci-permissions option.

The passphrase stays at version 1. Bootstrap uses the existing deployment lock
and checks noncurrent GCS generations as well as live state before creating a
key. It refuses existing encrypted state without its original key. Do not
delete the state secret while state or backups still need it. An environment
destroy does not remove these bootstrap resources, including the node account.
New Autopilot clusters use that account through their node autoprovisioning
defaults and omit the Google-computed cluster `nodeConfig`; Standard clusters
and node pools use their explicit node configuration. Existing clusters
keep their original node identity: bootstrap does not trigger an immutable
cluster replacement.
Use a fresh environment for migration; live legacy migration remains unverified.

Deploy reads the remote key into its Pulumi workspace. For an existing stack
or recovery, an explicit PULUMI_CONFIG_PASSPHRASE or
PULUMI_CONFIG_PASSPHRASE_FILE takes precedence. Bootstrap does not convert an
existing passphrase or rotate it. These paths have local regression coverage;
real project permissions and state decryption remain acceptance checks.


## Cloud SQL placement and capacity

New SQL instances prefer the first resolved `target.gcp.zones` entry. Existing
instances keep their current zone when the CLI is updated or the list is
reordered. Regional secondary placement stays Google-managed. See
[ADR 0017](adr/0017-gcp-sql-zone-preference.md).

A listed tier does not guarantee available capacity. After `ERROR_STOCKOUT`,
inspect the failed creation operation and current inventory. For a new instance,
put another compatible zone first and retry the same digest after the failed
operation is resolved. Preserve the state and resources already created.
The CLI does not retry this failure automatically.

MySQL 8.4 defaults to Enterprise Plus with `db-perf-optimized-N-2`. An explicit
dedicated-core tier such as `target.gcp.cloudSqlTier: db-custom-2-7680` selects
Enterprise while retaining MySQL 8.4. That example requests 2 vCPUs and 7680 MB
of memory. The provider validates custom size limits and regional dedicated-core
support; available capacity still depends on Google. Shared-core and N4 custom
tiers are outside this override. See [ADR 0018](adr/0018-gcp-explicit-sql-edition.md)
and [Google's edition overview](https://docs.cloud.google.com/sql/docs/mysql/editions-intro).

Keeping the existing tier keeps its edition. Changing an existing Plus database
to a dedicated custom tier can request a downgrade with downtime. Preview and
test that change outside production. The CLI does not select this alternative
automatically after a stockout.

## Review and apply GCP CI permissions

Prepare the account and inspect the returned ciPermissionPlan first:

```sh
magelift bootstrap --env test --github-owner YOUR_OWNER --github-repo YOUR_REPO
```

The build identity gets Artifact Registry writer on the owned repository. The
node identity gets repository reader. Deployment receives access to the state
bucket and key, node impersonation, and the service roles needed to provision
the test stack. The listed project roles apply across the selected GCP project;
they are not restricted to resources with a MageLift name. Review that scope
with the project administrator and prefer a dedicated test project.

Apply the reviewed grants and configure the nonsecret repository variables:

```sh
magelift bootstrap --env test --github-owner YOUR_OWNER --github-repo YOUR_REPO \
  --apply-ci-permissions --configure-github
magelift ci generate
```

The GitHub step verifies the immutable repository and owner IDs again before
writing. It reuses matching variables and refuses an existing different value.
It does not change workflow files, environment protection rules or repository
secrets. A partial setup can be rerun. Keep Composer credentials in the user's
GitHub secret settings when the shop needs them. Custom registry authentication
still follows the selected registry's documented flow.

These options extend the existing bootstrap request; the provider RPC operation
and SDK manifest remain unchanged. An older provider that cannot confirm the
grants is refused by the CLI. Local tests cover the request and IAM policies;
a real CI token exchange and deployment must be recorded separately.
