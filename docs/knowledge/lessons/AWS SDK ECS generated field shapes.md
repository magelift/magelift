---
type: lesson
title: AWS SDK ECS generated field shapes
description: In AWS SDK Go v2 ECS v1.88.1, Service desired/running/pending counts are int32 values, Deployment
  Status is a *string while RolloutState is an enum value, and Task LastStatus is a *string.
tags:
- go
- aws-sdk
- ecs
- testing
generated:
  at: '2026-07-24'
---

In AWS SDK Go v2 ECS v1.88.1, Service desired/running/pending counts are int32 values, Deployment Status is a *string while RolloutState is an enum value, and Task LastStatus is a *string. Do not assume all generated fields use pointers or enum constants.
