# Local AWS acceptance

Account-free CI uses Floci and Pulumi mocks. Real AWS acceptance is a **local,
opt-in maintainer activity**, not a GitHub Actions workflow. Use it sparingly to
stretch free credits: keep stacks up only for the duration of the script, default
to the `preview` preset, and destroy on exit.

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
once, then `acceptance cell-update` per incomplete cell — never destroy-between-cells.
Re-invoking with an existing checkpoint skips recorded cell IDs (ACCEPT-02).

**Resume / lock hygiene:** if a live (non-dry-run) deploy was killed mid-create, DIY
locks or `pending_operations` may block the next run. Unlock only when no deploy is
in flight (confirm with `magelift state status` / aws-cli) — never force-unlock while
create is still running. Live multi-cell proof (create-once, ≥3 cells, kill+resume,
dual `assert_clean`) closed 2026-07-29 on a disposable free-tier AWS account /
`eu-north-1`. Committed matrix sample:
[aws-matrix-results-sample-2026-07-29.md](evidence/aws-matrix-results-sample-2026-07-29.md)
(full local matrices stay gitignored under `.magelift/matrix-results.md`).

### Live multi-cell (paid)

When `MAGELIFT_ACCEPTANCE_DRY_RUN` is unset, the same script:

1. Runs one `acceptance create-once` (`preview` → `promote` → `deploy`) unless
   resume applies (`MAGELIFT_AWS_ACCEPTANCE_RESUME=1`, or checkpoint already has
   cells / first cell PASS).
2. Iterates `scripts/acceptance/cells-aws-preview.txt`: for each incomplete cell,
   logs `acceptance cell-update`, patches `target.aws.catalog.queueMode` (and the
   active env catalog) on a **temp copy** of `MAGELIFT_CONFIG` with `yq`, then
   redeploys the same digest — no destroy between cells.
3. Honors `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` (skip destroy on EXIT) for long-lived
   matrix / kill+resume; otherwise destroy + `assert_clean` on EXIT.

Broker cells need `queueSecretArn` already present in the acceptance config. Cells
`amazon-mq` / Aurora / OpenSearch are refused or absent from the catalog.

### assert_clean dual outcome (offline)

Shared helper: `scripts/acceptance/lib-assert-clean-aws.sh` (sourced by the EXIT
cleanup path after destroy unless `MAGELIFT_AWS_ACCEPTANCE_KEEP=true`).

Offline dual-outcome is proven with a PATH-isolated fake `aws` — never export the
stub in a live session:

```sh
MAGELIFT_ACCEPTANCE_AWS_STUB=1 bash tests/acceptance/assert_clean_stub_test.sh --clean
MAGELIFT_ACCEPTANCE_AWS_STUB=1 bash tests/acceptance/assert_clean_stub_test.sh --leftover
# leftover mode exits non-zero when leftovers are detected (expected)
```

Live leftover demonstration closed 2026-07-29: KEEP stack → `assert_clean FAILED`;
after `magelift destroy --yes` → `assert_clean ok`. Offline stubs remain the
default CI path.

## Prerequisites

- AWS credentials for a disposable account (aws-cli `aws sts get-caller-identity`)
- Pulumi available to the MageLift Automation API path used by `magelift deploy`
- An existing S3 access-log bucket in the target region
- Secrets Manager ARNs referenced by config, especially
  `target.aws.encryptionKeySecretArn`
- A signed immutable image digest and matching Cosign identity — **must** be a
  MageLift runtime image (`php-runtime` / `frankenphp-classic`) so `GET /health`
  returns 200 without Magento bootstrap (ALB + ECS container health)
- A `magelift.yaml` environment named for the profile you will run (`preview`
  recommended)

Unset `MAGELIFT_AWS_ENDPOINT_URL` so clients talk to real AWS, not Floci.

## Queue modes on acceptance

Default `preview` uses `queueMode: db` (no broker). To exercise self-hosted RabbitMQ
on free-tier-friendly spend, set `target.aws.catalog.queueMode: ecs-rabbitmq` and
provide `queueSecretArn`. Amazon MQ (`amazon-mq`) is the expensive cell — avoid it
on disposable acceptance accounts.

1. Prefer `preview` only. Standard and high-availability create Multi-AZ and
   managed-service spend that burns credits quickly.
2. Create resources, exercise the path, destroy in the same session.
3. Set a Budget alarm (for example $5 and $25) before the first run.
4. Do not leave NAT, OpenSearch, or Aurora running overnight.
5. Use `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` only while debugging; destroy before you
   stop for the day.

## Run

```sh
go build -trimpath -ldflags='-s -w' -o /tmp/magelift ./cmd/magelift

export MAGELIFT_BIN=/tmp/magelift
export MAGELIFT_CONFIG=/path/to/acceptance.magelift.yaml
export MAGELIFT_AWS_ACCEPTANCE_PROFILE=preview
export MAGELIFT_AWS_ACCEPTANCE_DIGEST='ghcr.io/example/app@sha256:…'
export MAGELIFT_AWS_CERTIFICATE_IDENTITY='…'
# optional override; defaults to https://token.actions.githubusercontent.com
# export MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER=…

# Bootstrap once per account/environment before the first acceptance pass:
# "$MAGELIFT_BIN" --config "$MAGELIFT_CONFIG" --env preview bootstrap \
#   --access-log-bucket existing-log-bucket \
#   --github-owner magelift --github-repo magelift

make aws-acceptance-local
```

The script runs `config validate`, `doctor`, `login`, then one create-once
(`preview` / `promote` / `deploy` / `outputs` / `health`) followed by catalog
cell-updates (temp YAML `queueMode` patch + redeploy). It **destroys on EXIT**
unless `MAGELIFT_AWS_ACCEPTANCE_KEEP=true`. Profiles other than `preview` require
`MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true`. Requires `yq` for live cell patches.

**Matrix sessions (recommended on free-tier):**

```sh
export MAGELIFT_AWS_ACCEPTANCE_KEEP=true
./scripts/aws-acceptance-local.sh
# kill mid-matrix, then resume:
export MAGELIFT_AWS_ACCEPTANCE_RESUME=1
./scripts/aws-acceptance-local.sh
# when fully done:
unset MAGELIFT_AWS_ACCEPTANCE_KEEP
magelift destroy --env preview --config "$MAGELIFT_CONFIG" --yes
```

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
