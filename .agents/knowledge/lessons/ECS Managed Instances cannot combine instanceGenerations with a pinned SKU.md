---
type: lesson
title: ECS Managed Instances cannot combine instanceGenerations with a pinned SKU
description: CreateCapacityProvider rejects instanceGenerations current/previous together with a generation-specific AllowedInstanceTypes value such as m6i.large.
tags: [aws, ecs, managed-instances, capacity-provider, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: aws-instance-requirements
    resource: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_InstanceRequirementsRequest.html
    title: InstanceRequirementsRequest
---

# ECS Managed Instances cannot combine instanceGenerations with a pinned SKU

`CreateCapacityProvider` Managed Instances `instanceRequirements` accepts
optional `instanceGenerations` (`current` | `previous`) and optional
`allowedInstanceTypes`. Combining `current`/`previous` with a
generation-specific type (`m6i.large`, `c5*`, `r5.xlarge`) returns
`ClientException`. AWS says to omit `instanceGenerations` or use a
generation-agnostic pattern (`m*.large`, `c*`).

Magento Managed Instances `20260813as` pinned `m6i.large` (spend cap) and
also set `InstanceGenerations: current`. Request
`711827ff-5a22-453a-b913-78d8d54cfaa4`. vCPU/memory bounds were already
present. EXIT destroyed the 94 created resources.

When the cell pins a concrete SKU, omit `instanceGenerations`. Do not switch
to `m*.large` just to keep `current`; that can select a larger/newer SKU.

# Related

* Relates to: [ECS Managed Instances capacity providers bind to the cluster at create](ECS%20Managed%20Instances%20capacity%20providers%20bind%20to%20the%20cluster%20at%20create.md)
* Relates to: [ECS Managed Instances instanceRequirements require vCPU and memory bounds](ECS%20Managed%20Instances%20instanceRequirements%20require%20vCPU%20and%20memory%20bounds.md)
* Relates to: [ECS capacity provider names cannot start with aws, ecs, or fargate](ECS%20capacity%20provider%20names%20cannot%20start%20with%20aws%2C%20ecs%2C%20or%20fargate.md)
