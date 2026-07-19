# AWS bootstrap

`magelift bootstrap` prepares the AWS account used by an environment. It verifies
the active caller account before changing anything, then creates the Pulumi S3
state bucket, its KMS key, a recovery-only state identity, and a GitHub Actions
infrastructure identity. It also creates a separate build identity that can read
only MageLift-scoped Composer and build secrets.

The access log bucket must already exist. MageLift does not create or delete that
shared logging bucket. State backups under `backups/` expire after 90 days, and
incomplete multipart uploads are removed after seven days. Current Pulumi state
objects are retained by versioning and are not covered by the backup expiry rule.

```sh
magelift bootstrap --env staging \
  --access-log-bucket existing-log-bucket \
  --github-owner acourtiol \
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

## Account-free contract tests

Run `make floci-test` when Docker is available. The pinned [Floci](https://github.com/floci-io/floci)
image exercises
KMS key reconciliation, S3 state setup and versioned-media restore, conditional lock
acquisition, ownership diagnostics, lock release, ECS candidate registration, Secrets
Manager version updates, and CloudWatch Logs ingestion without an AWS account.

Floci is an emulator, not an AWS certification environment. Its current IAM
surface does not implement GitHub OIDC-provider operations, and managed-service
behavior is not proof of AWS behavior. Identity bootstrap and managed-service
behavior still need a sparse local real-AWS acceptance pass when credits allow;
see [Local AWS acceptance](aws-acceptance.md).

For local AWS-compatible testing, set `MAGELIFT_AWS_ENDPOINT_URL` to the emulator
endpoint, for example `http://127.0.0.1:4566`. The endpoint must use HTTP or HTTPS
and resolve to loopback with no credentials or path. Bootstrap S3/KMS clients, the
state archive, and Secrets Manager/SSM clients honor it; S3 clients use path-style
requests. Production runs leave the variable unset.
