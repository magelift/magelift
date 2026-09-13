---
type: lesson
title: GCP Magento destroy must pass --destroy-backups
description: Pulumi destroy leaves Cloud SQL FINAL backups; magelift destroy without --destroy-backups is not leftover-deletion evidence.
tags:
- gcp
- backup
- destroy
- openspec
status: stable
generated:
  by: cursor-grok/desktop
  at: '2026-08-18'
---

`magelift destroy` keeps Cloud SQL backups that survive instance deletion unless
`--destroy-backups` is set. SQL-only leftover deletion is not a Pulumi cell.

The GCP Magento harness used to call `magelift destroy --yes` only. Standard and
HA presets retain `cloud-sql-final-backup`, so that destroy reported retained
backups and left billed leftovers. `assert_clean` listed instances, not backups.

`--destroy-backups` deletes GCP Cloud SQL leftovers for the Magelift instance
even when the backup policy is disposable (preview/zonal). AWS disposable
destroy stays a no-op; AWS retained snapshots are still refused.

Pass `--destroy-backups` on Magento destroy by default
(`MAGELIFT_GCP_DESTROY_BACKUPS=0` to keep leftovers). Inventory remaining
backups for `naming.CloudSQLInstance(project, environment)` (`NAME-PROFILE-sql`).
`force_clean_orphans` may sweep the same instance-scoped leftovers after a PSA
race; that harness sweep is not Pulumi evidence.

The 2026-08-18 HA Magento cell `gcha23` recorded `"destroyBackups": true`,
deleted the leftover FINAL Cloud SQL backup, and EXIT `assert_clean` reported
`remaining=0`. That is the `--destroy-backups` path on a Pulumi Magento stack,
not production destroy-without-flag retention.
