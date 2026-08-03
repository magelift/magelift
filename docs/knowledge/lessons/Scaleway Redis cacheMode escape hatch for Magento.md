---
type: lesson
title: Scaleway Redis cacheMode escape hatch for Magento
description: Scaleway experimental MageLift adapter uses Managed Redis with explicit cacheMode=redis because
  Scaleway has no managed Valkey.
tags:
- scaleway
- valkey
- redis
- magento
generated:
  by: cursor/wsl
  at: '2026-07-20T00:59:21.514935000+00:00'
---

Scaleway experimental MageLift adapter uses Managed Redis with explicit cacheMode=redis because Scaleway has no managed Valkey. Magento 2.4.9+ prefers Valkey; document the escape hatch and revisit when Scaleway ships Valkey. Pulumi provider is community pulumiverse/pulumi-scaleway — pin versions.

# Related

* Relates to: Projects/magelift/Lessons/MageLift OVH and Scaleway dual experimental providers
