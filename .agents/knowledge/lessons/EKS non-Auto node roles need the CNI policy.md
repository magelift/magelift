---
type: lesson
title: EKS non-Auto node roles need the CNI policy
description: Managed and self-managed EKS node roles must authorize the aws-node CNI daemonset to call the EC2 APIs it uses during bootstrap.
tags: [aws, eks, iam, kubernetes, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-17
---

# EKS non-Auto node roles need the CNI policy

Amazon EKS managed node groups and self-managed EC2 nodes run the `aws-node`
CNI daemonset with the node IAM role. If that role omits
`arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy`, bootstrap can create an EC2
instance without producing a usable Kubernetes node. The characteristic
signals are an `aws-node` `CrashLoopBackOff`, `MissingIAMPermissions` events
for EC2 network-interface operations, a `NotReady` node, and pending CoreDNS or
workload pods.

Auto Mode follows a different managed policy path and should not be fixed by
blindly adding the legacy CNI policy to its node role. Keep the policy matrix
explicit in code and test all three EC2/Auto branches. A live failure can
leave the EKS node group in `CREATING`; delete that exact node group and wait
for its instance to disappear before retrying the parent stack.

For the separate storage-controller identity boundary, see [the EBS CSI
pod-identity lesson](EKS%20EBS%20CSI%20add-ons%20need%20a%20pod%20identity%20role.md).
