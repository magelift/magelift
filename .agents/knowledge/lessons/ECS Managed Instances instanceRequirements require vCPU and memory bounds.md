---
type: lesson
title: ECS Managed Instances instanceRequirements require vCPU and memory bounds
description: CreateCapacityProvider Managed Instances rejects instanceRequirements that only list AllowedInstanceTypes; vCpuCount and memoryMiB are required.
tags: [aws, ecs, managed-instances, capacity-provider, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: aws-instance-requirements
    resource: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_InstanceRequirementsRequest.html
    title: InstanceRequirementsRequest
  - id: aws-create-mi-capacity-provider
    resource: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/create-capacity-provider-managed-instances.html
    title: Creating a capacity provider for Amazon ECS Managed Instances
---

# ECS Managed Instances instanceRequirements require vCPU and memory bounds

`CreateCapacityProvider` for Managed Instances takes an
`instanceLaunchTemplate.instanceRequirements` object. AWS documents
`vCpuCount` and `memoryMiB` as **required** (minimum at least). Allowed
instance types, generations, and other filters are optional.

Magento Managed Instances `20260813ar` set `AllowedInstanceTypes: m6i.large`
and `InstanceGenerations: current` only. The Pulumi AWS provider failed
before the API call (`memory_mib` and `vcpu_count` required). Create left 94
resources; EXIT destroy evaluated the same invalid program and could not
delete them. AWS CLI teardown emptied the inventories.

Pin min=max to the selected SKU so the cell cannot pick a larger type
(`m6i.large` → 2 vCPU / 8192 MiB). `AllowedInstanceTypes` is not a substitute.
EC2 Auto Scaling uses a launch template AMI instead of this struct; Fargate
does not use it.

# Related

* Relates to: [ECS Managed Instances cannot combine instanceGenerations with a pinned SKU](ECS%20Managed%20Instances%20cannot%20combine%20instanceGenerations%20with%20a%20pinned%20SKU.md)
* Relates to: [ECS capacity provider names cannot start with aws, ecs, or fargate](ECS%20capacity%20provider%20names%20cannot%20start%20with%20aws%2C%20ecs%2C%20or%20fargate.md)
* Relates to: [ECS EC2 capacity requires a pinned ami-* instanceAMI](ECS%20EC2%20capacity%20requires%20a%20pinned%20ami-%2A%20instanceAMI.md)
