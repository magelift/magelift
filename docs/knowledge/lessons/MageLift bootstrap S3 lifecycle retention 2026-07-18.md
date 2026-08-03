---
type: lesson
title: MageLift bootstrap S3 lifecycle retention 2026-07-18
description: Bootstrap now reconciles S3 versioning and lifecycle rules for Pulumi state.
tags:
- aws
- s3
- bootstrap
- retention
- backup
status: stable
generated:
  at: '2026-07-24'
---

Bootstrap now reconciles S3 versioning and lifecycle rules for Pulumi state. Completed snapshots under backups/ expire after 90 days, noncurrent backup versions expire after 90 days, and incomplete multipart uploads are aborted after 7 days. The policy is covered by race tests and Floci bootstrap tests. This retention is a bounded default and should remain documented if changed.
