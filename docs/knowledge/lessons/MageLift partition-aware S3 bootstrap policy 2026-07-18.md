---
type: lesson
title: MageLift partition-aware S3 bootstrap policy 2026-07-18
description: Bootstrap identity policies must derive S3 bucket ARNs from the AWS partition.
tags:
- bootstrap
- iam
- aws-partitions
- security
generated:
  at: '2026-07-24'
---

Bootstrap identity policies must derive S3 bucket ARNs from the AWS partition. Hard-coding arn:aws:s3 breaks GovCloud and China regions even when OIDC, KMS, and SSM ARNs are partition-aware. BuildIdentityPlan now uses arn:<partition>:s3 and tests GovCloud and China.
