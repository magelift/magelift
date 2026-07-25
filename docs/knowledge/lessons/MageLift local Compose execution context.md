---
type: lesson
title: MageLift local Compose execution context
description: The CLI now provides magelift dev init/up/down/reset/status/logs/exec.
tags:
- local-development
- docker-compose
- security
- frankenphp
generated:
  at: '2026-07-24'
---

The CLI now provides magelift dev init/up/down/reset/status/logs/exec. It generates .magelift/compose.local.yml with digest-pinned MySQL 8.4 and Valkey 8.1 defaults, loopback-only host ports, named volumes, and an optional app profile using the FrankenPHP classic local image. dev reset requires --yes.
