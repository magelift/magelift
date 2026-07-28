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
locks or `pending_operations` may block the next run. Unlock only when the account
is empty of acceptance resources (confirm with `assert_clean` / aws-cli tag scan) —
never force-unlock while create is still in flight. Live multi-cell proof remains a
paid HUMAN_GATE pass; dry-run does not claim that yet.

## Prerequisites

- AWS credentials for a disposable account (aws-cli `aws sts get-caller-identity`)
- Pulumi available to the MageLift Automation API path used by `magelift deploy`
- An existing S3 access-log bucket in the target region
- Secrets Manager ARNs referenced by config, especially
  `target.aws.encryptionKeySecretArn`
- A signed immutable image digest and matching Cosign identity
- A `magelift.yaml` environment named for the profile you will run (`preview`
  recommended)

Unset `MAGELIFT_AWS_ENDPOINT_URL` so clients talk to real AWS, not Floci.

## Efficient credit use

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
#   --github-owner acourtiol --github-repo magelift

make aws-acceptance-local
```

The script runs `config validate`, `doctor`, `login`, `preview`, `promote`,
`deploy`, `outputs`, and `health --mode runtime`, then **destroys on EXIT** unless
`MAGELIFT_AWS_ACCEPTANCE_KEEP=true`. Profiles other than `preview` require
`MAGELIFT_AWS_ACCEPTANCE_ALLOW_COSTLY=true`.

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
