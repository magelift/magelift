---
type: lesson
title: nginx readonly rootfs needs /var/log/nginx tmpfs
description: nginx opens /var/log/nginx/error.log before applying nginx.conf error_log /dev/stderr.
tags:
- nginx
- ecs
- e2e
- readonly
status: stable
generated:
  at: '2026-07-24'
---

nginx opens /var/log/nginx/error.log before applying nginx.conf error_log /dev/stderr. With ECS readonlyRootFilesystem that open alerts. Locally nginx still runs; still mount tmpfs for /var/log/nginx and /var/cache/nginx on the web container. Separately: acceptance image may lack /app/pub. Web service e2e failed TaskFailedToStart (web exit 1, varnish 137); migration/deploy and cron succeeded after secret host-key fix.
