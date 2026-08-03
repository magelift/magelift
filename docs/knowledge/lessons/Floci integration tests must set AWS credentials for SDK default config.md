---
type: lesson
title: Floci integration tests must set AWS credentials for SDK default config
description: The Floci ECS smoke test initially set static credentials only on a dedicated ECS client,
  but operations.NewRuntime loads the default AWS config independently.
tags:
- floci
- aws-sdk
- testing
generated:
  at: '2026-07-24'
---

The Floci ECS smoke test initially set static credentials only on a dedicated ECS client, but operations.NewRuntime loads the default AWS config independently. Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the process before constructing adapters so account-free endpoint tests do not fall back to EC2 IMDS.
