---
type: lesson
title: MageLift nginx non-root config test required 2026-07-18
description: Adding nginx to the Debian PHP runtime initially failed its non-root nginx -t check because
  nginx created default uwsgi and scgi temp directories under /var/lib/nginx.
tags:
- nginx
- containers
- security
- failure
generated:
  at: '2026-07-24'
---

Adding nginx to the Debian PHP runtime initially failed its non-root nginx -t check because nginx created default uwsgi and scgi temp directories under /var/lib/nginx. Configure every temp path under /tmp for read-only root filesystem tasks.
