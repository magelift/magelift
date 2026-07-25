---
type: lesson
title: ECS web /health 404 blocks varnish dependsOn HEALTHY
description: 'Integrated ECS tasks: varnish dependsOn web HEALTHY; web healthCheck is curl -sf http://127.0.0.1:8080/health.'
tags:
- ecs
- varnish
- health
- nginx
- acceptance
generated:
  by: cursor/wsl
  at: '2026-07-19T22:56:33.327308000'
---

Integrated ECS tasks: varnish dependsOn web HEALTHY; web healthCheck is curl -sf http://127.0.0.1:8080/health. If the Magento image nginx.conf lacks location = /health, nginx returns 404, curl -f fails, web stays UNHEALTHY, varnish stays PENDING forever, ALB TG empty, deploy stabilize never completes. Acceptance image sha256:162122… lacked /health; sha256:df04554… includes it. Always bake /health into the runtime image before e2e; verify with docker run curl against the digest.
