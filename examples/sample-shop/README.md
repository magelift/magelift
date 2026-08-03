# Sample Magento shop (starter YAML)

This folder is a **configuration sketch** for a Magento Open Source project on
the certified AWS ECS Fargate path (GCP GKE Autopilot is also certified; use
its docs when targeting GCP). It is not a full Magento codebase. Copy it beside
an existing Magento repo (or Composer create-project), then fill secrets and
accounts.

## Goal

Reach a cloud **preview** URL in under an hour once bootstrap is done, using the
`preview` preset and database-backed queues (no Amazon MQ spend).

## What to replace in `magelift.yaml`

| Field / area | Replace with |
| --- | --- |
| `project` / Magento name | Your shop slug (resource prefix) |
| `environments.*.account` | Your AWS account ID (or GCP project via target block) |
| Domains / hostnames | Hosts you control; never commit personal test domains |
| Secret ARNs / Secret Manager refs | Your Composer auth, crypt key, DB secrets |
| `--access-log-bucket` | An S3 (or equivalent) bucket in the target region |
| `--github-owner` / `--github-repo` | The GitHub repo that will assume deploy roles |

Do **not** paste plaintext Composer tokens or Magento crypt keys into YAML.

## Steps

1. Copy `magelift.yaml` into your Magento repository root and complete the table
   above.
2. Prefer `target.aws.catalog.queueMode: ecs-rabbitmq` on non-preview presets when
   Amazon MQ cost is a concern ([capability matrix](../../docs/capability-matrix.md)).
3. Follow [getting started](../../docs/getting-started.md), then:

```sh
magelift doctor
magelift bootstrap --env preview --access-log-bucket YOUR_LOG_BUCKET \
  --github-owner YOUR_ORG --github-repo YOUR_REPO
magelift config validate --env preview
magelift preview --env preview
magelift deploy --env preview --yes
magelift outputs --env preview
```

4. Optional: measure wall clock with `scripts/time-to-preview.sh` after bootstrap.

## Coming from Upsun / ACC

Read [migrating from PaaS](../../docs/migrating-from-paas.md). Map variables to
Secrets Manager references; do not paste plaintext secrets into YAML.

## Headless storefronts

Set `application.mode: headless` when Magento is API-only. Deploy Next.js / PWA
with your usual tool; point it at Magento outputs (`applicationURL`, media URL).
See [storefront recipes](../../docs/storefront-recipes.md).
