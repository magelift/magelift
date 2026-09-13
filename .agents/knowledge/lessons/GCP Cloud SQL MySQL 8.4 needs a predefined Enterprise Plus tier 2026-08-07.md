---
type: lesson
title: GCP Cloud SQL MySQL 8.4 needs a predefined Enterprise Plus tier
description: Release-aware MySQL 8.4 selection must also select an Enterprise Plus machine tier that Cloud SQL accepts.
tags: [gcp, cloud-sql, magento, acceptance, pulumi]
status: stable
---

# Rule

When MageLift selects Cloud SQL `MYSQL_8_4`, it must set the instance edition to
`ENTERPRISE_PLUS` and use a predefined `db-perf-optimized-N-*` or `db-c4a-*`
tier. The current Europe West 1 acceptance default is
`db-perf-optimized-N-2`. The deployment plan and account-free cost estimate
must use the same resolver.

# Failure

The first corrected 2.4.9 standard run, `m249s3`, selected the right database
version but kept the previous `db-custom-2-7680` tier. Cloud SQL rejected the
instance during the Pulumi apply:

```text
Invalid Tier (db-custom-2-7680) for (ENTERPRISE_PLUS) Edition. Use a predefined Tier like db-perf-optimized-N-* instead.
```

The run stopped before Magento resources or application cells were created.

# Fix

The GCP database component now resolves the edition and default tier from the
database version. Planning rejects a custom tier for MySQL 8.4 before resource
creation, and the cost estimator calls the same default-tier function.

# Verification

The focused GCP suite covers the resolver, invalid custom overrides, the
Pulumi mock graph, and the release-aware cost estimate. The next live standard
and high-availability runs must verify both `MYSQL_8_4` and the explicit
Enterprise Plus tier before their evidence can be promoted to certification.

The rule is based on the [Cloud SQL MySQL database versions
documentation](https://docs.cloud.google.com/sql/docs/mysql/db-versions) and
the [Cloud SQL instance creation
documentation](https://docs.cloud.google.com/sql/docs/mysql/create-instance).
