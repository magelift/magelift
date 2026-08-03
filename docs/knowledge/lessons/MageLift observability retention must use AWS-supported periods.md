---
type: lesson
title: MageLift observability retention must use AWS-supported periods
description: CloudWatch Logs accepts a fixed set of retention periods.
tags:
- aws
- cloudwatch
- observability
status: stable
generated:
  at: '2026-07-24'
---

CloudWatch Logs accepts a fixed set of retention periods. The observability component validates the requested period before resource registration and protects production log groups from destroy.
