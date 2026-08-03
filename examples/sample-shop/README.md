# Sample Magento shop (starter YAML)

This folder is a **configuration sketch** for a Magento Open Source project on
the certified AWS ECS Fargate path. It is not a full Magento codebase — add it
beside an existing Magento repo (or Composer create-project), then fill secrets
and accounts.

## Goal

Reach a cloud **preview** URL in under an hour once bootstrap is done, using the
`preview` preset and database-backed queues (no Amazon MQ spend).

## Steps

1. Copy `magelift.yaml` into your Magento repository root and edit project name,
   accounts, domains, and secret ARNs.
2. Prefer `target.aws.catalog.queueMode: ecs-rabbitmq` on non-preview presets when
   Amazon MQ cost is a concern ([capability matrix](../../docs/capability-matrix.md)).
3. Run:

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
