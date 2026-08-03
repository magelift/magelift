---
type: lesson
title: MageLift Varnish tmpfs hardening 2026-07-18
description: The integrated Varnish 8.0.2 ECS sidecar now keeps its root filesystem read-only and receives
  only an executable task-scoped tmpfs at /var/lib/varnish with uid/gid 1000, mode 0750, and 384 MiB.
tags:
- aws
- ecs
- varnish
- security
- failure
- runtime
generated:
  at: '2026-07-24'
---

The integrated Varnish 8.0.2 ECS sidecar now keeps its root filesystem read-only and receives only an executable task-scoped tmpfs at /var/lib/varnish with uid/gid 1000, mode 0750, and 384 MiB. A first 64 MiB tmpfs smoke test failed with VSM errno 28; 384 MiB with VARNISH_SIZE=256M started successfully. A zsh verification wrapper also failed when it used the readonly variable name status and left one manual smoke container that was removed afterward; use rc for shell result variables and always verify cleanup. The runtime graph test asserts the tmpfs contract.
