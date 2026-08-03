---
type: lesson
title: MageLift candidate task updates all application containers 2026-07-18
description: Candidate ECS task registration must replace the promoted digest in every container sharing
  the application image, including nginx and PHP-FPM, while preserving unrelated sidecars.
tags:
- aws
- ecs
- deployment
- supply-chain
generated:
  at: '2026-07-24'
---

Candidate ECS task registration must replace the promoted digest in every container sharing the application image, including nginx and PHP-FPM, while preserving unrelated sidecars. The previous code changed only ContainerDefinitions[0], which could leave a multi-container runtime split across digests. Test covers two application containers plus a Varnish sidecar.
