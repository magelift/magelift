---
type: lesson
title: ECS Managed Instances CreateCapacityProvider races IAM policy attachment
description: RolePolicyAttachment can return success before the assumed ECS infrastructure role can call ec2:DescribeInstanceTypeOfferings; CreateCapacityProvider must wait, not only DependsOn.
tags: [aws, ecs, managed-instances, capacity-provider, iam, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-23
sources:
  - id: aws-iam-eventually-consistent
    resource: https://docs.aws.amazon.com/IAM/latest/UserGuide/troubleshoot_general.html#troubleshoot_general_eventual-consistency
    title: IAM eventual consistency
  - id: aws-managed-mi-infra-policy
    resource: https://docs.aws.amazon.com/aws-managed-policy/latest/reference/AmazonECSInfrastructureRolePolicyForManagedInstances.html
    title: AmazonECSInfrastructureRolePolicyForManagedInstances
---

# ECS Managed Instances CreateCapacityProvider races IAM policy attachment

`AmazonECSInfrastructureRolePolicyForManagedInstances` already allows
`ec2:DescribeInstanceTypeOfferings`. A 400 `ClientException` from
`CreateCapacityProvider` that says the assumed
`.../ECSManagedInstances` role is not authorized for that action is IAM
propagation, not a missing policy.

`aws:iam:RolePolicyAttachment` can complete in well under a second.
ECS then assumes the infrastructure role immediately. STS evaluations
can still see the role without the managed policy.

`pulumi.DependsOn` the attachments is necessary graph order. It is not
enough. Soak after the attachment IDs are known (skip Pulumi preview and
`go test`), then create the capacity provider.

# Related

* Relates to: [ECS Managed Instances capacity providers bind to the cluster at create](ECS%20Managed%20Instances%20capacity%20providers%20bind%20to%20the%20cluster%20at%20create.md)
* Relates to: [ECS Managed Instances instanceRequirements require vCPU and memory bounds](ECS%20Managed%20Instances%20instanceRequirements%20require%20vCPU%20and%20memory%20bounds.md)
