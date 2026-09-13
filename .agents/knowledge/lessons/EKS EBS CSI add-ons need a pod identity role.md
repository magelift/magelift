---
type: lesson
title: EKS EBS CSI add-ons need a pod identity role
description: The EKS EBS CSI controller must receive its EC2 permissions through an explicit service-account identity; a broad node-role attachment does not replace IRSA when pod metadata access is unavailable.
tags: [aws, eks, iam, kubernetes, ebs, csi, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-17
---

# EKS EBS CSI add-ons need a pod identity role

The managed `aws-ebs-csi-driver` add-on calls EC2 APIs from its controller
pod. Configure a stack-owned IAM OIDC provider for the cluster, a dedicated
role with `sts:AssumeRoleWithWebIdentity` restricted to
`system:serviceaccount:kube-system:ebs-csi-controller-sa`, the required EBS
managed policy, and the role ARN on the EKS add-on's `serviceAccountRoleArn`.

The EC2 node role is not a substitute for this binding. If pod access to IMDS
is disabled or unavailable, the controller cannot obtain the instance role and
the add-on fails with `no EC2 IMDS role found`, `failed to refresh cached
credentials`, or `DescribeAvailabilityZones` probe errors. Giving every pod on
the node the EBS policy is also a larger trust boundary than the controller's
service account requires. Keep the EBS policy on the dedicated role and keep
the non-Auto node role limited to worker, image-pull, and CNI responsibilities.

The `aws-ebs-csi-driver` add-on can remain in a create/waiting state while the
node group and `aws-node` CNI are healthy, so a successful node bootstrap does
not prove storage readiness. Verify the add-on status, controller pod
credentials, `ebs.csi.aws.com` StorageClass behavior, and a disposable volume
operation separately. Record any startup-only cron signal before teardown so
it is not mistaken for an EBS identity failure.
