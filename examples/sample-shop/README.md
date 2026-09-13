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

AWS laptop bootstrap also needs an existing log bucket (`--access-log-bucket`).
GitHub owner/repo flags are only for Actions CI.

Do **not** paste plaintext Composer tokens or Magento crypt keys into YAML.

## Steps

1. Copy `magelift.yaml` into your Magento repository root and complete the table
   above.
2. Prefer `target.aws.catalog.queueMode: ecs-rabbitmq` on non-preview presets when
   Amazon MQ cost is a concern ([capability matrix](../../docs/capability-matrix.md)).
3. Follow [getting started](../../docs/getting-started.md), then stay on Magelift
   commands:

```sh
magelift doctor
magelift bootstrap --env preview --access-log-bucket YOUR_LOG_BUCKET
magelift deploy --env preview --yes
magelift health --mode runtime
magelift destroy --env preview --yes
```

GCP omits `--access-log-bucket`. Do not set a state-backend URL.

4. Optional: measure wall clock with `scripts/time-to-preview.sh` after bootstrap.

## Pull-request CI previews

From the repository containing this file, generate and validate the workflow at a
released MageLift version:

```sh
magelift ci generate --magelift-version v1.0.0-rc.1
magelift ci validate --magelift-version v1.0.0-rc.1
```

Apply the `magelift-preview` label to a pull request to run its preview. The workflow
uses the GitHub repository and pull-request number for the environment identity, so
new commits and branch renames reuse the same stack. Close cleanup carries the run
generation and refuses a stale event before it can destroy a newer deployment.

For AWS, set the role ARN, region, image, and environment variables created by
`magelift bootstrap --github-owner --github-repo`. For GCP, configure
`MAGELIFT_GCP_PROJECT_ID`, `MAGELIFT_GCP_REGION`,
`MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER`, and
`MAGELIFT_GCP_SERVICE_ACCOUNT`. Magelift derives stack state after bootstrap.
The GCP workflow uses GitHub federation and short-lived credentials; do not add
a service-account JSON key.

## Coming from Upsun / ACC

Read [migrating from PaaS](../../docs/migrating-from-paas.md). Map variables to
Secrets Manager references; do not paste plaintext secrets into YAML.

## Headless storefronts

Set `application.mode: headless` when Magento is API-only. Deploy Next.js / PWA
with your usual tool; point it at Magento outputs (`applicationURL`, media URL).
See [storefront recipes](../../docs/storefront-recipes.md).
