---
type: lesson
title: MageLift AWS service compatibility gates use current Adobe tables
description: The AWS stack planner checks Magento 2.4.9 and 2.4.8 against OpenSearch 3, Valkey 8.x, AWS
  MQ RabbitMQ 3.13, and Aurora MySQL 3.11 or 3.12; Magento 2.4.7 and 2.4.6 allow OpenSearch 2 or 3 with
  the ...
tags:
- compatibility
- adobe
- aws
- catalog
generated:
  at: '2026-07-24'
---

The AWS stack planner checks Magento 2.4.9 and 2.4.8 against OpenSearch 3, Valkey 8.x, AWS MQ RabbitMQ 3.13, and Aurora MySQL 3.11 or 3.12; Magento 2.4.7 and 2.4.6 allow OpenSearch 2 or 3 with the same AWS managed-service floor. This target policy is separate from the Magento/PHP catalog and can be bypassed only with compatibility.allowUnsupported, which records unsupported-allowed metadata. Values were checked against the Adobe system requirements page on 2026-07-18.
