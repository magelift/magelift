---
type: lesson
title: MageLift ports and adapters for multi-provider stacks 2026-07-19
description: Shared Magento ports live in internal/platform (StackModule, RequiredOutputKeys, CoreEnvBindings,
  workloads).
tags:
- gcp
- multi-cloud
- ports
- pulumi
status: stable
generated:
  by: cursor/wsl
  at: '1784487838735104'
---

Shared Magento ports live in internal/platform (StackModule, RequiredOutputKeys, CoreEnvBindings, workloads). Cloud adapters under internal/cloud/<provider> own product topology. Do not share Pulumi Network/Database components across clouds (ADR 0002/0008). CLI selects StackModule by provider+runtime; AWS remains certified; GCP gcp/gke-autopilot is experimental with VPC+Cloud SQL+Memorystore Valkey+GKE Autopilot. Output keys applicationURL/databaseWriter/cacheEndpoint/networkVpcId/clusterName/serviceName/privateSubnetIds are required for every module.
