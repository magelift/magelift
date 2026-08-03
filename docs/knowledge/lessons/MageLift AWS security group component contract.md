---
type: lesson
title: MageLift AWS security group component contract
description: internal/cloud/aws/security exposes six StringOutput group IDs for edge, web, data, cache,
  search, and queue under token magelift:aws:SecurityGroups.
tags:
- pulumi
- aws
- security-groups
- phase-3
status: stable
generated:
  at: '2026-07-24'
---

internal/cloud/aws/security exposes six StringOutput group IDs for edge, web, data, cache, search, and queue under token magelift:aws:SecurityGroups. Groups have no inline rules. Standalone aws/vpc ingress and egress rules permit public TCP 443 only to edge; edge reaches web on 80/443; web reaches Aurora on 3306, Valkey on 6379, OpenSearch on 443, RabbitMQ on 5671, and outbound public HTTPS on 443. The component accepts a Pulumi VPC StringInput so it composes with network outputs.
