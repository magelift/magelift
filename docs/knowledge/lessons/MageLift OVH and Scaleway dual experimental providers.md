---
type: lesson
title: MageLift OVH and Scaleway dual experimental providers
description: Both OVH (ovh/mks) and Scaleway (scaleway/kapsule) ship as first-party experimental adapters
  copying the GCP K8s-shaped pattern.
tags:
- ovh
- scaleway
- pulumi
- experimental
- french
generated:
  by: cursor/wsl
  at: '2026-07-20T00:58:40.436838000+00:00'
---

Both OVH (ovh/mks) and Scaleway (scaleway/kapsule) ship as first-party experimental adapters copying the GCP K8s-shaped pattern. Validated with Pulumi WithMocks only — no OVH/Scaleway accounts. OVH uses official pulumi-ovh + native Valkey. Scaleway uses pulumiverse/pulumi-scaleway + explicit cacheMode redis escape hatch (no managed Valkey). Day-2 ports return ErrNotSupported. Required Magento output keys exported. Worktree branch feat/ovh-scaleway-experimental.
