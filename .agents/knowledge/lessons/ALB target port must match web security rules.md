---
type: lesson
title: ALB target port must match web security rules
description: The ingress target group and ECS container use port 8080.
tags:
- aws
- security
- ecs
- ingress
generated:
  at: '2026-07-24'
---

The ingress target group and ECS container use port 8080. Security group rules that only allow 80 and 443 from the ALB prevent traffic from reaching tasks. Shared security rules must use the configured target port.
