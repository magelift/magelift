---
type: lesson
title: RDS ManageMasterUserPassword secret only has username password
description: AWS RDS/Aurora ManageMasterUserPassword secrets contain only username and password JSON keys
  — not host, port, or dbname.
tags:
- rds
- secrets
- ecs
- e2e
status: stable
generated:
  at: '2026-07-24'
---

AWS RDS/Aurora ManageMasterUserPassword secrets contain only username and password JSON keys — not host, port, or dbname. Verified on free-tier RDS MySQL e2e: secret keys=[password, username]. ECS JSON key selectors for host fail with ResourceInitializationError. MageLift must inject MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST/PORT/DBNAME as plain env from WriterEndpoint + 3306 + Dependencies.DatabaseName, and only pull username/password from the managed secret.
