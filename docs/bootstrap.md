# Bootstrap

`magelift bootstrap` prepares the selected cloud account so Magento can deploy.
Laptop users omit GitHub flags. GitHub Actions needs `--github-owner` and
`--github-repo` so Magelift can create the CI identity.

You do not set a state-backend URL. Magelift derives it from `magelift.yaml`
after bootstrap.

## Laptop (certified AWS or GCP)

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

Generate the workflow from the same configuration that will be deployed:

```sh
magelift ci generate --magelift-version v1.0.0-rc.1
magelift ci validate --magelift-version v1.0.0-rc.1
```

The generator selects the registered provider/runtime adapter. AWS ECS Fargate uses
the role variables created by `magelift bootstrap --github-owner --github-repo`.
GCP GKE uses GitHub federation variables Magelift prints after that same bootstrap:
`MAGELIFT_GCP_PROJECT_ID`, `MAGELIFT_GCP_REGION`,
`MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER`, and
`MAGELIFT_GCP_SERVICE_ACCOUNT`. Magelift derives stack state from `magelift.yaml`
after bootstrap; do not set a state-backend URL. Also provide the image
references and the environment-specific preview/staging/production variables
required by the generated workflow.

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
