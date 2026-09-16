---
type: lesson
title: CloudFront CachingDisabled policy constraints 2026-07-19
description: Custom CloudFront cache policies with Min/Default/Max TTL all 0 (caching disabled) reject
  CookieBehavior/QueryStringBehavior other than none and reject EnableAcceptEncodingGzip/Brotli true.
tags:
- aws
- cloudfront
- kms
status: stable
generated:
  at: '2026-07-24'
---

Custom CloudFront cache policies with Min/Default/Max TTL all 0 (caching disabled) reject CookieBehavior/QueryStringBehavior other than none and reject EnableAcceptEncodingGzip/Brotli true. Use managed CachingDisabled policy ID 4135ea2d-6df8-44a3-9df3-4b5a84be39ad and forward cookies/query via OriginRequestPolicy. Media CDN managed CachingOptimized ID is 658327ea-f89d-4fab-a63d-7e88639e58f6 (trailing 6 required). Bootstrap KMS keys need logs.<region>.amazonaws.com in key policy for encrypted CloudWatch Log Groups. Stack keys too: a manually created stack key with only the default root policy fails the first encrypted resource with AccessDeniedException ("not allowed to be used with Arn 'arn:aws:logs:...'"). CloudWatch Logs requires an explicit key-policy statement for the logs.<region>.amazonaws.com service principal — a documented AWS special case (RDS, ElastiCache, and Secrets Manager work via caller grants). Fix shape: Sid AllowCloudWatchLogsUseOfKey scoped by ArnLike kms:EncryptionContext:aws:logs:arn to the run's log groups.
