---
type: lesson
title: GCP Memorystore Valkey requires Service Connection Policy
description: Memorystore for Valkey DesiredAutoCreatedEndpoints (PSC service connectivity automation)
  fails with "No service connection policy is associated with project/network/region" unless a networkconnecti...
tags:
- gcp
- memorystore
- valkey
- networkconnectivity
- psc
generated:
  by: cursor/wsl
  at: '2026-07-19T19:18:12.929726000+00:00'
---

Memorystore for Valkey DesiredAutoCreatedEndpoints (PSC service connectivity automation) fails with "No service connection policy is associated with project/network/region" unless a networkconnectivity.ServiceConnectionPolicy exists first. ServiceClass must be gcp-memorystore (not gcp-memorystore-redis). PscConfig.subnetworks must list the regional private subnets; Location matches the instance region. Enable networkconnectivity.googleapis.com before real up. MageLift creates the SCP in internal/cloud/gcp/cache/cache.go before memorystore.Instance.
