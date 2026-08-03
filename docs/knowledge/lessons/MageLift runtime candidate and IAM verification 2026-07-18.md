---
type: lesson
title: MageLift runtime candidate and IAM verification 2026-07-18
description: The AWS ECS deploy task now runs app:config:import, setup:upgrade, cache:clean, and cache:flush
  in one fail-fast command before durable service update.
tags:
- deployment
- iam
- floci
- verification
status: stable
generated:
  at: '2026-07-24'
---

The AWS ECS deploy task now runs app:config:import, setup:upgrade, cache:clean, and cache:flush in one fail-fast command before durable service update. Runtime and deploy containers receive the Aurora managed database secret reference as MAGELIFT_DATABASE_CREDENTIALS. A separate execution-role policy scopes Secrets Manager read and KMS decrypt through the regional Secrets Manager service. Pulumi mock tests, go test -race, make verify, and make floci-test passed on 2026-07-18. Floci covers ECS registration and health only, not managed AWS services or IAM.
