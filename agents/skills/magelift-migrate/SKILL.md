---
name: magelift-migrate
description: >-
  Import Magento settings from Adobe Commerce Cloud, Upsun, Platform.sh or
  Compose and rehearse a migration to AWS, GCP or Azure.
version: 1.1.0
---

# Migrate to MageLift

Run `magelift init` in the shop checkout. Choose `--from-upsun`, `--from-acc` or `--from-compose`, and select the destination with `--provider aws`, `gcp` or `azure`. For multi-app Upsun, use `--app` to select the Magento backend. Use `--config-out rehearsal.php` to keep a separate file for review.

- Read the generated PHP and the adjacent unmapped report. Exit status 2 after the file is written means some settings need review. Check hooks, redirects, mounts, reader routing, cache backends, Fastly configuration and jobs.
- Compare the lockfile, source PHP branch and Composer plugins. A source tool version such as `2.9.8` imports as `2.9.8+`. Keep licensed dependencies and Composer credentials available to the builder.
- Check session storage, eviction policy, OpenSearch analyzers, RabbitMQ consumers, patch IDs and theme locales. Review every single-replica data service planned for UAT or production.
- Transfer the source Magento encryption key through the destination's secret store before restoring a database. Keep PR data and credentials isolated.
- Rehearse SQL and media recovery with a consistent source snapshot. On GKE or AKS, `magelift migrate restore` shows a plan by default. Check the context, namespace UID, database identity, credentials and TLS before using `--execute --yes`. Use the appropriate media store for the provider.
- Check Admin, checkout, reindexing, media and cron on the restored site. For a headless shop, test authentication, cookies and the storefront's GraphQL requests from its actual origin.

Record the write freeze, final sync, DNS switch, rollback owner and treatment of orders placed after cutover. [Migration guide](https://magelift.dev/docs/migrating-from-paas/) · [Data rehearsal](https://magelift.dev/docs/weekend-migrate/)
