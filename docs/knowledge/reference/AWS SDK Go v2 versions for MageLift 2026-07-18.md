---
type: knowledge
title: AWS SDK Go v2 versions for MageLift 2026-07-18
description: Context7 official AWS SDK Go v2 documentation confirms config.LoadDefaultConfig(ctx), Secrets
  Manager GetSecretValue(ctx, input) with SecretString or SecretBinary, and the requirement not to log
  se...
tags:
- aws-sdk-go-v2
- dependencies
- secrets
generated:
  at: '2026-07-24'
sources:
- id: context7-aws-aws-sdk-go-v2-and-go-list-m-latest
  resource: Context7 /aws/aws-sdk-go-v2 and go list -m @latest
---

Context7 official AWS SDK Go v2 documentation confirms config.LoadDefaultConfig(ctx), Secrets Manager GetSecretValue(ctx, input) with SecretString or SecretBinary, and the requirement not to log sensitive output. Official Go module resolution on 2026-07-18 reports core v1.42.1, config v1.32.30, secretsmanager v1.43.1, and ssm v1.72.0. Context7 library metadata lagged at core v1.39.0, so module versions were verified against the Go proxy.
