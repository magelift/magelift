---
type: lesson
title: MageLift runtime compatibility and Varnish boundary 2026-07-18
description: Adobe system requirements updated 2026-06-01 list nginx 1.30 and Varnish 8 for Commerce 2.4.9.
tags:
- magento
- compatibility
- nginx
- varnish
- runtime
- failure
status: stable
generated:
  at: '2026-07-24'
---

Adobe system requirements updated 2026-06-01 list nginx 1.30 and Varnish 8 for Commerce 2.4.9. MageLift now copies the pinned official nginx 1.30.4 Debian image into the PHP runtime, labels and tests the version, and uses a pinned official Varnish 8.0.2 sidecar for integrated ECS tasks. Integrated traffic targets Varnish port 6081 and forwards to nginx port 8080; headless traffic targets nginx directly. The Varnish sidecar keeps a writable root because varnishd requires its VSM working directory on an executable filesystem and it contains no application secrets. An initial image build failed because the NGINX stage was inserted before the PHP extension RUN; moving the stage after that RUN fixed it. A read-only Varnish smoke test failed because VSM cannot use a noexec tmpfs, so the sidecar exception is intentional.
