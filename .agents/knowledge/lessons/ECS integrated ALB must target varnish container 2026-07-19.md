---
type: lesson
title: ECS integrated ALB must target varnish container 2026-07-19
description: For applicationMode=integrated, ECS Service LoadBalancers.ContainerName must be varnish (port
  6081), not web.
tags:
- aws
- ecs
- varnish
status: stable
generated:
  at: '2026-07-24'
---

For applicationMode=integrated, ECS Service LoadBalancers.ContainerName must be varnish (port 6081), not web. The nginx/frankenphp container has no host port mapping in integrated mode; Varnish owns 6081. AWS CreateService rejects ContainerName=web with ContainerPort=6081: container web did not have a container port 6081 defined. Latent until first free-tier deploy reached ECS service create.
