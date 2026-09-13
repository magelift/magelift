---
type: lesson
title: ECS EC2 capacity requires a pinned ami-* instanceAMI
description: computeMode ec2-asg fails plan admission with an empty AMI; pin an ECS-optimized ami-* from the public SSM parameter, do not omit instanceAmi.
tags: [aws, ecs, ec2, ami, acceptance]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-13
sources:
  - id: aws-ecs-ami
    resource: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/retrieve-ecs-optimized_AMI.html
    title: Retrieving Amazon ECS-optimized Linux AMI metadata
---

# ECS EC2 capacity requires a pinned ami-* instanceAMI

`target.aws.catalog.fargate.computeMode: ec2-asg` still uses the ECS Fargate
runtime identity, but host capacity is an Auto Scaling group with a launch
template. `instanceType` is not enough. `instanceAmi` must be a live `ami-*`
in the target region.

Magento EC2 ASG `20260813ao` set `t3.medium` and left `instanceAmi` empty.
`magelift login` failed at plan admission (`AWS plan contains an empty AMI`)
with `created=0`. The runtime path would have failed the same way
(`ECS EC2 capacity requires a pinned ami-* instanceAMI`).

Pin the current Amazon ECS-optimized Amazon Linux 2023 AMI from public SSM
`/aws/service/ecs/optimized-ami/amazon-linux-2023/recommended/image_id` in the
same region as the stack. Match architecture to the instance type (`t3.medium`
is x86_64, not arm64). Fargate and Fargate Spot do not need this field.

# Related

* Relates to: [ACM DomainValidationOptions JSON can exist before ResourceRecord](ACM%20DomainValidationOptions%20JSON%20can%20exist%20before%20ResourceRecord.md)
