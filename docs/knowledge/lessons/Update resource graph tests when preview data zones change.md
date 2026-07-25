---
type: lesson
title: Update resource graph tests when preview data zones change
description: On 2026-07-18 the preview network graph changed from one to two zones to satisfy Aurora DB
  subnet-group requirements.
tags:
- pulumi
- network
- aurora
- tests
- failure
generated:
  at: '2026-07-24'
---

On 2026-07-18 the preview network graph changed from one to two zones to satisfy Aurora DB subnet-group requirements. The focused snapshot then failed on resource ordering, subnet CIDR count, and endpoint route-table count. The test now sorts expected names and asserts the six subnet CIDRs and four endpoint route tables explicitly.
