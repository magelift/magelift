---
type: lesson
title: Runtime test must select web service by name
description: The runtime target-group test selected the first ECS service resource, which was not guaranteed
  to be the web service when Pulumi mock registration order varied.
tags:
- testing
- pulumi
- runtime
- determinism
generated:
  at: '2026-07-24'
---

The runtime target-group test selected the first ECS service resource, which was not guaranteed to be the web service when Pulumi mock registration order varied. Selecting shop-web-service by name removes the nondeterministic nil loadBalancers panic.
