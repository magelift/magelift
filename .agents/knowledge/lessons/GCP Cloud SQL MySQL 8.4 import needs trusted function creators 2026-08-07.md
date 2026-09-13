---
type: lesson
title: GCP Cloud SQL MySQL 8.4 import needs trusted function creators
description: Magento dumps with stored functions fail on binary-logged Cloud SQL MySQL unless log_bin_trust_function_creators is enabled.
tags: [gcp, cloud-sql, mysql, magento, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-07
---

# Failure

The first live 2.4.9 standard GKE run created Cloud SQL MySQL 8.4 correctly
with the Enterprise Plus tier, passed the infrastructure checks, and then
failed while importing the real B2B dump:

```text
ERROR 1419 (HY000) at line 846: You do not have the SUPER privilege and binary logging is enabled
```

The importer pod was healthy and its transport path was working. The error
came from MySQL while creating a stored function, so retrying the Kubernetes
exec path would only waste paid runtime.

# Fix

The GCP Cloud SQL component now enables the boolean database flag
`log_bin_trust_function_creators=on` whenever it creates the Magento MySQL
instance. The focused GCP tests cover the flag in the Pulumi resource graph.
The failed live run was torn down and remains excluded from certification
until the corrected 2.4.9 stack completes the real dump import and candidate
cells.

# Related

AWS RDS already sets the equivalent flag. Keep this setting in every provider
adapter that imports Magento dumps into a binary-logged MySQL service.
