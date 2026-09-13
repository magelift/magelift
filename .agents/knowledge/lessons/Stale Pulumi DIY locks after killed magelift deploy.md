---
type: lesson
title: Stale Pulumi DIY locks after killed magelift deploy
description: Killing magelift mid-update leaves S3 DIY locks under .pulumi/locks/ and Magelift deploy
  lock.
tags:
- pulumi
- s3
- locks
- acceptance
generated:
  at: '2026-07-24'
---

Killing magelift mid-update leaves S3 DIY locks under .pulumi/locks/ and Magelift deploy lock. Clear with aws s3 rm on locks prefix + magelift state unlock --yes. Interrupted creates may need refresh; orphaned ALBs can block VPC destroy.
