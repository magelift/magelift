---
type: lesson
title: ECS Managed Instances capacity providers bind to the cluster at create
description: CreateCapacityProvider for Managed Instances takes cluster and auto-associates; PutClusterCapacityProviders is unsupported and races before ACTIVE.
tags: [aws, ecs, managed-instances, capacity-provider, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: aws-put-cluster-capacity-providers
    resource: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_PutClusterCapacityProviders.html
    title: PutClusterCapacityProviders
  - id: aws-create-mi-capacity-provider
    resource: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/create-capacity-provider-managed-instances.html
    title: Creating a capacity provider for Amazon ECS Managed Instances
---

# ECS Managed Instances capacity providers bind to the cluster at create

`CreateCapacityProvider` for Managed Instances requires `cluster`. AWS then
makes that provider available only on that cluster. `PutClusterCapacityProviders`
is documented as unsupported for Managed Instances.

Magento Managed Instances `20260813at` created `ml-awsat-preview-runtime-managed`
in 0.83s, then Pulumi `ClusterCapacityProviders` called
`PutClusterCapacityProviders` and got `InvalidParameterException`: the provider
was not yet ACTIVE (request `1d4e28cd-4e96-42b9-8530-b983b335280b`). A sleep
does not fix this: the API is the wrong association path.

Skip cluster association for Managed Instances. Keep the ECS service
`capacityProviderStrategy`. Fargate and EC2 Auto Scaling still use
`PutClusterCapacityProviders`.

`CreateService` with that strategy has a different race: AWS can return
from `CreateCapacityProvider` before `status=ACTIVE`. Magento `awsmi`
then got `InvalidParameterException: The capacity provider specified in
capacity provider strategy is not ACTIVE` one second later. Soak after
the provider ID is known, then create the services. A sleep is the
wrong fix for `PutClusterCapacityProviders`; it is the right fix for
`CreateService`.

# Related

* Relates to: [ECS Managed Instances cannot combine instanceGenerations with a pinned SKU](ECS%20Managed%20Instances%20cannot%20combine%20instanceGenerations%20with%20a%20pinned%20SKU.md)
* Relates to: [ECS Managed Instances CreateCapacityProvider races IAM policy attachment](ECS%20Managed%20Instances%20CreateCapacityProvider%20races%20IAM%20policy%20attachment.md)
* Relates to: [ECS Managed Instances instanceRequirements require vCPU and memory bounds](ECS%20Managed%20Instances%20instanceRequirements%20require%20vCPU%20and%20memory%20bounds.md)
