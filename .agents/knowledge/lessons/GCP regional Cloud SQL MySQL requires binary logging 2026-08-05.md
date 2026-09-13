---
type: lesson
title: GCP regional Cloud SQL MySQL requires binary logging
description: Regional Cloud SQL for MySQL creation fails unless binary logging is enabled.
tags:
- gcp
- cloud-sql
- mysql
- acceptance
status: stable
generated:
  by: codex/desktop
  at: '2026-08-05'
sources:
- id: gcp-cloud-sql-ha
  resource: https://docs.cloud.google.com/sql/docs/mysql/configure-ha?hl=en
  title: Configure high availability for Cloud SQL for MySQL
---

The first live standard-profile run failed while creating the regional Cloud
SQL instance. GCP returned: `MySQL HA non-replica instances need to have
binary logging enabled`.

MageLift now sets `binaryLogEnabled: true` for regional MySQL and enables the
regional backup configuration. The resource graph test covers this so every
provider adapter that offers a regional MySQL shape can apply the same
requirement before spending time on a live deployment.
