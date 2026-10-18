---
type: lesson
title: EKS self-managed AL2023 user data must be MIME NodeConfig
description: EKS-optimized AL2023 nodes parse an application/node.eks.aws MIME part before user-data scripts, so a shell script cannot stand in for the required cluster NodeConfig.
tags: [aws, eks, al2023, nodeadm, kubernetes, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-17
sources:
  - id: aws-node-bootstrapping
    resource: https://docs.aws.amazon.com/eks/latest/eksctl/node-bootstrapping.html
    title: AWS EKS node bootstrapping
  - id: aws-al2023
    resource: https://docs.aws.amazon.com/eks/latest/userguide/al2023.html
    title: AWS EKS AL2023 documentation
  - id: self-managed-pass
    resource: ../../../docs/evidence/aws-eks-self-managed-magento-live-20260817bf.md
    title: AWS EKS self-managed Magento live baseline 20260817bf
---

# EKS self-managed AL2023 user data must be MIME NodeConfig

An EKS-optimized Amazon Linux 2023 AMI starts `nodeadm-config` before the
cloud-init user-data script. The launch template must therefore provide a
MIME multipart user-data document containing an `application/node.eks.aws`
part with a `NodeConfig`. For self-managed nodes and launch templates, that
part must include the cluster name, API server endpoint, base64 certificate
authority, and Kubernetes service CIDR. `nodeadm-run` completes later after
the user-data phase.

Putting a shell script at the top level of AL2023 user data and having that
script write the complete NodeConfig is too late. The characteristic failure
is an EC2 instance that is `running`/`InService` but never registers with
EKS; the console reports `nodeadm-config` YAML decode errors such as `mapping
values are not allowed in this context`, while CoreDNS and workload Jobs stay
Pending. This can look like a subnet, CNI, or IAM failure until the EC2
console output is checked.

Keep the MIME NodeConfig as the primary path. If the same renderer must still
support older AL2 EKS AMIs, include a `text/x-shellscript` part that invokes
`/etc/eks/bootstrap.sh` only when `nodeadm` is unavailable. Do not rerun
`nodeadm init` from user data on AL2023; AWS warns that doing so can break
boot ordering and ENI assumptions.

The corrected renderer was then proven in a fresh cold cell: two AL2023
instances across two Availability Zones registered `Ready`, the system plane
and EBS CSI add-on became healthy, and an EBS PVC/Job completed a write/read
assertion. The pass remains a create/runtime/storage baseline; it does not
prove node-loss or zone-loss recovery.

See [the corrected live baseline](../../../docs/evidence/aws-eks-self-managed-magento-live-20260817bf.md),
the [EKS EBS CSI identity lesson](EKS%20EBS%20CSI%20add-ons%20need%20a%20pod%20identity%20role.md),
and the [non-Auto CNI lesson](EKS%20non-Auto%20node%20roles%20need%20the%20CNI%20policy.md).
