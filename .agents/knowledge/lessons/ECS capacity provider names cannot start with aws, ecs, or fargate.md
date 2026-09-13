---
type: lesson
title: ECS capacity provider names cannot start with aws, ecs, or fargate
description: CreateCapacityProvider rejects names prefixed with aws, ecs, or fargate; project tags like awsap produce invalid names unless rewritten.
tags: [aws, ecs, capacity-provider, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
sources:
  - id: aws-create-capacity-provider
    resource: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CreateCapacityProvider.html
    title: CreateCapacityProvider
---

# ECS capacity provider names cannot start with aws, ecs, or fargate

`CreateCapacityProvider` allows letters, numbers, underscores, and hyphens,
up to 255 characters, and **rejects** names prefixed with `aws`, `ecs`, or
`fargate` (case-insensitive).

Magento EC2 ASG `20260813ap` used project tag `awsap`, so the provider name
was `awsap-preview-runtime-ec2`. AWS returned `ClientException` before the
stack finished creating. Fargate and Fargate Spot did not hit this because
they use the built-in `FARGATE` / `FARGATE_SPOT` providers.

Prefix reserved names with `ml-` (`ml-awsap-preview-runtime-ec2`). Do not
fix this by renaming the MageLift project tag alone; any `aws*` / `ecs*` /
`fargate*` stack name will fail the same way. Managed Instances uses the
same `Name` field.

# Related

* Relates to: [ECS EC2 capacity requires a pinned ami-* instanceAMI](ECS%20EC2%20capacity%20requires%20a%20pinned%20ami-%2A%20instanceAMI.md)
* Relates to: [ECS Managed Instances instanceRequirements require vCPU and memory bounds](ECS%20Managed%20Instances%20instanceRequirements%20require%20vCPU%20and%20memory%20bounds.md)
