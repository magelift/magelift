---
type: lesson
title: MageLift current AWS service compatibility 2026-07-18
description: Adobe system requirements snapshot updated 2026-06-01 lists current Magento lines 2.4.9 PHP8.5,
  2.4.8 PHP8.4/8.3, 2.4.7 PHP8.3/8.2, 2.4.6 PHP8.2/8.1, with 2.4.5 and 2.4.4 PHP8.1 only.
tags:
- compatibility
- aws
- adobe
- 2026-07-18
status: stable
generated:
  at: '2026-07-24'
---

Adobe system requirements snapshot updated 2026-06-01 lists current Magento lines 2.4.9 PHP8.5, 2.4.8 PHP8.4/8.3, 2.4.7 PHP8.3/8.2, 2.4.6 PHP8.2/8.1, with 2.4.5 and 2.4.4 PHP8.1 only. AWS MQ supports RabbitMQ 3.13 and 4.2; RabbitMQ 4.2 requires mq.m7g. ElastiCache supports Valkey 8.x and 9.x; MageLift allows Valkey 9 only for Magento 2.4.9 and 8.x for earlier supported lines. OpenSearch 3.x remains valid for latest lines. Planner tests current service versions before Pulumi resource registration.
