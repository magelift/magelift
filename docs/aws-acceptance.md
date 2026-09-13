# Local AWS acceptance

Catalog ownership: OpenSpec `certification-aws`. Shared evidence rules stay in
`certification-matrix`. Certified AWS Magento today is the ECS Fargate preview
tuple in the [capability matrix](capability-matrix.md) plus
[evidence](evidence/README.md). Packed KEEP after this tracker is applied still uses the existing
harness files. Fargate packed catalog `awsba` is closed (destroy plus
`assert_clean`). Cold Managed Instances `awsmi` and EKS Auto Mode `awsek`
are closed the same way and stay experimental.

Account-free CI uses Floci AWS, floci-gcp, and Pulumi mocks (`make local-gates`).
Real AWS acceptance is a **local, opt-in maintainer activity**, not a GitHub
Actions workflow. Use it sparingly: light smoke only, keep stacks up only for
the duration of the script, default to the `preview` preset, and destroy on exit.

## Offline harness (dry-run)

Before any paid create, prove resume and evidence append with zero AWS spend:

```sh
export MAGELIFT_ACCEPTANCE_DRY_RUN=1
make acceptance-harness-test
# or: ./scripts/aws-acceptance-local.sh
```

| Artifact | Path |
|----------|------|
| Cell catalog | `scripts/acceptance/cells-aws-preview.txt` |
| Checkpoint | `.magelift/acceptance-checkpoint.json` (gitignored) |
| Evidence table | `.magelift/matrix-results.md` (gitignored; SC#2 columns) |
| Shared libs | `scripts/acceptance/lib-checkpoint.sh`, `lib-evidence.sh` |

Dry-run iterates the catalog against one logical stack: logs `acceptance create-once`
once, then `acceptance cell-update` per incomplete cell. Never destroy-between-cells.
Re-invoking with an existing checkpoint skips recorded cell IDs (ACCEPT-02).

**Resume / lock hygiene:** if a live (non-dry-run) deploy was killed mid-create, DIY
locks or `pending_operations` may block the next run. Unlock only when no deploy is
in flight (confirm with `magelift state status` / aws-cli). Never force-unlock while
create is still running. Live multi-cell proof (create-once, ≥3 cells, kill+resume,
dual `assert_clean`) closed 2026-07-29 on a disposable free-tier AWS account /
`eu-north-1`. Committed matrix sample:
[aws-ecs-fargate-magento-live-20260813ai.md](evidence/aws-ecs-fargate-magento-live-20260813ai.md)
(full local matrices stay gitignored under `.magelift/matrix-results.md`).

### Live multi-cell (paid)

When `MAGELIFT_ACCEPTANCE_DRY_RUN` is unset, the same script:

1. Runs one `acceptance create-once` (`preview` → `promote` → `deploy`) unless
   resume applies (`MAGELIFT_AWS_ACCEPTANCE_RESUME=1`, or checkpoint already has
   cells / first cell PASS).
2. Iterates `scripts/acceptance/cells-aws-preview.txt`: for each incomplete cell,
   logs `acceptance cell-update`, patches `target.aws.catalog.queueMode` (and the
   active env catalog) on a **temp copy** of `MAGELIFT_CONFIG` with `yq`, then
   redeploys the same digest. No destroy between cells.
3. Honors `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` (skip destroy on EXIT) for long-lived
   matrix / kill+resume; otherwise destroy + `assert_clean` on EXIT.

Broker cells need `queueSecretArn` already present in the acceptance config. Cells
`amazon-mq` / Aurora / OpenSearch are refused or absent from the catalog.

### EKS matrix sessions

EKS uses the same warm-session lifecycle, but its cell catalog changes the
Kubernetes workload rather than the ECS service definition:

| Profile | Catalog | Default cells |
|---------|---------|---------------|
| `preview` | `scripts/acceptance/cells-aws-eks-preview.txt` | Database-backed queue, no search |
| `standard` | `scripts/acceptance/cells-aws-eks-standard.txt` | RabbitMQ or database queue, OpenSearch or no search |
| `high-availability` | `scripts/acceptance/cells-aws-eks-high-availability.txt` | HA RabbitMQ/database queue, HA OpenSearch or no search |

Set `target.runtime: eks` in the acceptance config. The harness
patches `target.aws.catalog.eks.queueMode` and
`target.aws.catalog.eks.searchMode` on a temporary config copy, so the EKS
cluster, VPC, database, and Valkey stack are reused across compatible cells.
Every EKS catalog begins with an explicit `computeMode:<mode>` cell. That cell
must match `target.aws.catalog.eks.computeMode` (default `auto-mode`) and is
recorded as the create-once architecture boundary; the harness refuses to
change it during a warm session. Run the same profile separately for
`auto-mode`, `managed-node-groups`, `self-managed`, and `fargate`, using a
mode-specific config and matching catalog. The ready-to-run architecture-only
catalogs are `scripts/acceptance/cells-aws-eks-architecture-*.txt`; they keep
the first cold cell deliberately small (`database` queue and disabled search).
Architecture changes are cold boundaries, while queue/search changes remain
eligible for warm reuse. For standard or high-availability service cells, use
the matching mode-specific config and append those profile's queue/search
cells to a copied catalog; do not reuse a PASS cell from another compute mode.
For an architecture-only cold run, select the catalog explicitly:

```sh
MAGELIFT_ACCEPTANCE_CELL_CATALOG=scripts/acceptance/cells-aws-eks-architecture-managed-node-groups.txt \
MAGELIFT_AWS_ACCEPTANCE_PROFILE=preview \
./scripts/aws-acceptance-local.sh
```

Preview does not run the ECS seed task because Magento runs in Kubernetes.
Standard and HA require the EKS catalog's managed-service settings, including
a distinct session secret when Valkey authentication is enabled.

ECS capacity modes have the same cold-boundary rule. Use one of
`scripts/acceptance/cells-aws-ecs-architecture-fargate.txt`,
`cells-aws-ecs-architecture-fargate-spot.txt`,
`cells-aws-ecs-architecture-ec2-asg.txt`, or
`cells-aws-ecs-architecture-managed-instances.txt`, with a matching
`target.aws.catalog.fargate.computeMode`. Fargate and Fargate Spot use the
same task-definition contract but have different interruption evidence;
EC2-ASG and Managed Instances additionally require host/capacity-provider
evidence. A warm session may change queue cells only after the architecture
baseline for that exact mode has passed.

### assert_clean dual outcome (offline)

Shared helper: `scripts/acceptance/lib-assert-clean-aws.sh` (sourced by the EXIT
cleanup path after destroy unless `MAGELIFT_AWS_ACCEPTANCE_KEEP=true`).

Offline dual-outcome is proven with a PATH-isolated fake `aws`. Never export the
stub in a live session:

```sh
MAGELIFT_ACCEPTANCE_AWS_STUB=1 bash tests/acceptance/assert_clean_stub_test.sh --clean
MAGELIFT_ACCEPTANCE_AWS_STUB=1 bash tests/acceptance/assert_clean_stub_test.sh --leftover
# leftover mode exits non-zero when leftovers are detected (expected)
```

Live leftover demonstration closed 2026-07-29: KEEP stack → `assert_clean FAILED`;
after `magelift destroy --yes` → `assert_clean ok`. Offline stubs remain the
default CI path.

The latest paid preview pass covered the database, ECS RabbitMQ, and ECS ActiveMQ
Artemis queue cells on one Magento 2.4.9 ECS Fargate stack. See the
[2026-08-06 evidence](evidence/aws-ecs-fargate-magento-live-20260813ai.md). The EXIT
cleanup deletes versioned Pulumi state in batches of at most 1,000 objects because
that is the S3 `DeleteObjects` limit.

## Prerequisites

The acceptance entrypoints validate local command dependencies before any AWS
resource is created. See [acceptance command dependencies](acceptance-dependencies.md)
for the shared `jq`/Mike Farah `yq` v4 preflight and the no-auto-install policy.

- AWS credentials for a disposable account (aws-cli `aws sts get-caller-identity`)
- Pulumi available to the MageLift Automation API path used by `magelift deploy`
- An existing S3 access-log bucket in the target region
- Secrets Manager ARNs referenced by config, especially
  `target.aws.encryptionKeySecretArn`
- A signed immutable image digest and matching Cosign identity. **Must** be a
  MageLift runtime image (`php-runtime`, nginx + PHP-FPM) so `GET /health`
  returns 200 without Magento bootstrap (ALB + ECS container health)
- A `magelift.yaml` environment named for the profile you will run (`preview`
  recommended)

Unset `MAGELIFT_AWS_ENDPOINT_URL` so clients talk to real AWS, not Floci.

When a live run pre-creates disposable Secrets Manager values, pass their
exact ARNs as a newline- or comma-separated
`MAGELIFT_AWS_ACCEPTANCE_PREREQUISITE_SECRET_ARNS` value. Each secret must be
tagged with the exact `magelift:project`,
`magelift:acceptance-run=magelift-acceptance-<project>-<profile>`,
`magelift:managed-by=magelift`, and `magelift:purpose=acceptance` values. The
cleanliness gate refuses every other acceptance-named secret. Set
`MAGELIFT_AWS_ACCEPTANCE_DELETE_PREREQUISITE_SECRETS=true` only when those
exact ARNs are disposable values created for this run; the EXIT cleanup then
force-deletes them after the stack destroy. User-owned or shared secrets must
not be placed in this allowlist.

The harness owns its derived S3 Pulumi backend and waits for both a successful
`HeadBucket` and `ListObjectsV2` call before invoking Pulumi. This avoids
starting a paid create while the new bucket is still converging through an
AWS endpoint.

## Queue modes on acceptance

Default `preview` uses `queueMode: db` (no broker). To exercise self-hosted RabbitMQ
on free-tier-friendly spend, set `target.aws.catalog.queueMode: ecs-rabbitmq` and
provide `queueSecretArn`. Amazon MQ (`amazon-mq`) is the expensive cell. The default preview catalog
excludes it. A packed KEEP campaign may opt in with
`MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true` and
`scripts/acceptance/cells-aws-campaign-fargate-keep.txt` on a three-AZ origin
YAML. That catalog also warm-patches ECS `searchMode` (`disabled` then
`serverless`), `databaseEngine` (`rds-mysql` then `aurora-mysql`), and
`ha:multi-az`. Do not mark those cells certified. Destroy at campaign close.

1. Prefer `preview` only. Standard and high-availability create Multi-AZ and
   managed-service spend that burns credits quickly.
2. Create resources, exercise the path, destroy in the same session.
3. Set a Budget alarm (for example $5 and $25) before the first run.
4. Do not leave NAT, OpenSearch, or Aurora running overnight.
5. Use `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` only while debugging; destroy before you
   stop for the day.
6. The live harness assigns a six-hour disposable TTL by default. Override it with
   `MAGELIFT_ACCEPTANCE_TTL_SECONDS` (maximum 24 hours); the watchdog forces the
   EXIT cleanup path at expiry and overrides `KEEP=true`.

## Run

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=1GiB \
  go build -trimpath -ldflags='-s -w' -o /tmp/magelift ./cmd/magelift

export MAGELIFT_BIN=/tmp/magelift
export MAGELIFT_CONFIG=/path/to/acceptance.magelift.yaml
export MAGELIFT_AWS_ACCEPTANCE_PROFILE=preview
export MAGELIFT_AWS_ACCEPTANCE_DIGEST='ghcr.io/example/app@sha256:…'
export MAGELIFT_CERTIFICATE_IDENTITY='…'
# optional alias: MAGELIFT_AWS_CERTIFICATE_IDENTITY
# optional override; defaults to https://token.actions.githubusercontent.com
# export MAGELIFT_CERTIFICATE_OIDC_ISSUER=…
# optional non-interactive sign before promote:
# export MAGELIFT_COSIGN_IDENTITY_TOKEN_FILE=/path/to/jwt

# Optional when the account has more than one Pulumi backend. The harness does
# not inherit an ambient PULUMI_BACKEND_URL: if this variable is omitted, it
# derives the disposable S3 backend name from the account, region, and profile,
# and creates a temporary passphrase file inside its working directory.
# export MAGELIFT_AWS_ACCEPTANCE_BACKEND_URL='s3://…?region=eu-north-1&awssdk=v2'

# Bootstrap once per account/environment before the first acceptance pass:
# "$MAGELIFT_BIN" --config "$MAGELIFT_CONFIG" --env preview bootstrap \
#   --access-log-bucket existing-log-bucket \
#   --github-owner magelift --github-repo magelift

make aws-acceptance-local
```

The script runs `config validate`, `doctor`, `login`, then one create-once
(`preview` / optional `sign` / `promote` / `deploy` / `outputs` / `health`) followed by catalog
cell updates on a temporary YAML copy. Queue cells and EKS search cells use
`deploy --infra-only` after the first cell, then run bounded health checks, so
they do not rerun Magento's schema migration. It **destroys on EXIT** unless
`MAGELIFT_AWS_ACCEPTANCE_KEEP=true`. Profiles other than `preview` require
`MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true`. Requires `yq` for live cell patches.

**Matrix sessions (recommended on free-tier):**

```sh
export MAGELIFT_AWS_ACCEPTANCE_DIR=/tmp/magelift-aws-preview
export MAGELIFT_AWS_ACCEPTANCE_KEEP=true
./scripts/aws-acceptance-local.sh
# kill mid-matrix, then resume:
export MAGELIFT_AWS_ACCEPTANCE_RESUME=1
./scripts/aws-acceptance-local.sh
# when fully done:
export MAGELIFT_AWS_ACCEPTANCE_KEEP=false
unset MAGELIFT_AWS_ACCEPTANCE_RESUME
./scripts/aws-acceptance-local.sh
```

For EKS, use a separate workdir and the EKS profile catalog:

```sh
export MAGELIFT_AWS_ACCEPTANCE_DIR=/tmp/magelift-aws-eks-preview
export MAGELIFT_AWS_ACCEPTANCE_PROFILE=preview
export MAGELIFT_AWS_ACCEPTANCE_KEEP=true
./scripts/aws-acceptance-local.sh

# After all cells have completed, destroy the one warm stack.
export MAGELIFT_AWS_ACCEPTANCE_KEEP=false
unset MAGELIFT_AWS_ACCEPTANCE_RESUME
./scripts/aws-acceptance-local.sh
```

The same workdir keeps the Pulumi passphrase and generated config. The
acceptance ownership marker is stable for the project tag and profile, while
each evidence run still gets its own run ID. A configuration fingerprint keeps
old PASS cells from being reused after a release, digest, runtime, or catalog
change. Use a cold stack for a provider, engine, database major version, or
topology change. Warm reuse is for update-safe cells on the same stack.

The expiry boundary is a stranded-resource guard, not proof of cleanup. Every
completed session still runs `destroy`, bounded deletion polling, and direct
owning-service inventory assertions. A TTL-triggered run is never allowed to
retain a stack for debugging.

Avoid Amazon MQ, Aurora, NAT Gateway, and OpenSearch on free-tier accounts.

## aws-cli cleanup

If the exit trap fails, inspect and remove leftovers manually:

```sh
aws sts get-caller-identity
aws resourcegroupstaggingapi get-resources \
  --tag-filters Key=magelift:project,Values=<project> \
  --region "$AWS_REGION"
# Then magelift destroy --yes, or delete stranded resources by service.
```

Tag-based discovery depends on MageLift resource tags; always finish with
`magelift destroy` when state still exists.

## What this proves (and what it does not)

A green local preview pass proves bootstrap OIDC, Pulumi apply, ECS runtime health,
and destroy for that account and configuration. It does not certify every managed
service edge case, GitHub Actions OIDC from CI, or non-AWS providers. Floci remains
the daily offline regression harness. Community reports remain welcome for account
shapes maintainers do not run continuously.
