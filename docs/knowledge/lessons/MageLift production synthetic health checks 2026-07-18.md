---
type: lesson
title: MageLift production synthetic health checks 2026-07-18
description: MageLift production AWS observability now provisions a CloudWatch Synthetics canary using
  syn-nodejs-puppeteer-11.0.
tags:
- aws
- synthetics
- observability
- pulumi
- health
status: stable
generated:
  at: '2026-07-24'
---

MageLift production AWS observability now provisions a CloudWatch Synthetics canary using syn-nodejs-puppeteer-11.0. The canary checks the public HTTPS /health endpoint every five minutes, stores run artifacts in a dedicated versioned private SSE-KMS S3 bucket, uses a least-privilege execution role, and alarms on missing or failing SuccessPercent data. The script object uses a separate non-expiring prefix; the artifact prefix has lifecycle retention. Synthetic resources are enabled only for class production and can be tested with Pulumi mocks without AWS credentials.
