---
name: magelift-operate
description: Inspect a deployed MageLift shop, collect logs, promote an image, roll back, or clean up an environment.
version: 1.0.0
---

# Operate a shop

Use `--env` to select the exact shop environment in every script.

```sh
magelift health --env staging --mode config
magelift outputs --env staging
magelift health --env staging --mode runtime
magelift logs --env staging --service web
magelift exec --env staging --service web --container php-fpm -- bin/magento cache:status
```

Use the runtime health check after a deploy or rollback. For a failed
initial deployment, inspect the migration logs and keep the database for
diagnosis. MageLift initializes an empty database and reports a failure
if it encounters a partial installation.

Deploy by digest and keep the previous image digest for rollback.
Review the maintenance procedure before applying an incompatible schema
change to production. The rollback command redeploys an image. Restore
database and media backups separately. [Operations guide](https://magelift.dev/docs/operations/)

Use `magelift state` for state backups and restores. Keep the Pulumi
passphrase secret for as long as the state and its backups need it. A
preview may share the state storage and registry with its parent
environment. Destroy only the preview's resources. Use `magelift preview
prune` for expired previews and review any retained resources after teardown.

On Google Cloud, logs, outputs, tunnels, and runtime health can read an
existing stack without an image digest. For a Cloud SQL capacity error,
check whether an instance was created before retrying in another zone.
Keep the existing state and database. On Azure, check the state account,
tenant, and database TLS settings before deploying an existing shop.

When reporting an operation, include the environment, image digest, and
checks performed. Keep secret output and customer data out of logs shared
with others.
