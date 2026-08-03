---
type: lesson
title: MageLift AWS SDK v2 secret adapter versions 2026-07-18
description: As checked through Context7 official AWS SDK sources and the Go module proxy on 2026-07-18,
  use github.com/aws/aws-sdk-go-v2 v1.42.1, config v1.32.30, service/secretsmanager v1.43.1, and service/ss...
tags:
- aws
- secrets
- go-sdk
- versions
status: stable
generated:
  at: '2026-07-24'
---

As checked through Context7 official AWS SDK sources and the Go module proxy on 2026-07-18, use github.com/aws/aws-sdk-go-v2 v1.42.1, config v1.32.30, service/secretsmanager v1.43.1, and service/ssm v1.72.0. Load credentials and region with config.LoadDefaultConfig(ctx); add config.WithRegion only for a non-empty explicit region. Secrets Manager GetSecretValue returns decrypted SecretString or SecretBinary. SSM GetParameter must set WithDecryption=true. Keep SDK clients behind operation-specific interfaces for unit tests and never log returned values.
