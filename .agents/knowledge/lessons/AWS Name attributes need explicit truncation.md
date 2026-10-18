---
type: lesson
title: AWS Name attributes need explicit truncation
description: Pulumi auto-names and Magelift stack prefixes like acceptance-preview-* overflow AWS limits
  (ALB/TG 32, AOSS VPC endpoint 32, AOSS collection/group Magelift regex max 26).
tags:
- aws
- naming
- pulumi
- acceptance
generated:
  at: '2026-07-24'
---

Pulumi auto-names and Magelift stack prefixes like acceptance-preview-* overflow AWS limits (ALB/TG 32, AOSS VPC endpoint 32, AOSS collection/group Magelift regex max 26). Use internal/cloud/aws/naming.AWSName. Keep policy resource refs aligned with the truncated collection name.
