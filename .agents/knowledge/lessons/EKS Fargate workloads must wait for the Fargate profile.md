---
type: lesson
title: EKS Fargate workloads must wait for the Fargate profile
description: A Kubernetes Job created before the EKS Fargate profile is ACTIVE stays Pending on existing Fargate nodes and will not be rescheduled.
tags: [aws, eks, fargate, pulumi, kubernetes]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: eks-fargate
    resource: https://docs.aws.amazon.com/eks/latest/userguide/fargate.html
    title: AWS Fargate for Amazon EKS
---

# EKS Fargate workloads must wait for the Fargate profile

EKS Fargate admission mutates matching pods onto `fargate-scheduler`. That
webhook is not useful until the Fargate profile is ACTIVE. A Job created in
parallel with `aws.eks.FargateProfile` can admit a pod first; that pod then
sits Pending (`no nodes available`, then `untolerated taint` on CoreDNS
Fargate nodes). Deleting the pod lets the Job retry after the profile exists.

Magento EKS Fargate `20260813ay` hit this on `awsay-preview-runtime-db-grant`
(~8 min Pending). CoreDNS DeploymentPatch already `DependsOn` the profile;
the grant Job did not. Grant, web, cron, and any other default-namespace
workloads must `DependsOn` the Fargate profile (and the CoreDNS patch when
cluster DNS is required) so Pulumi does not race them.
