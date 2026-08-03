---
type: lesson
title: GKE Autopilot cluster name max 40 characters
description: gcp.container.Cluster name cannot exceed 40 characters.
tags:
- gcp
- gke
- naming
generated:
  by: cursor/wsl
  at: '1784504144441957'
---

gcp.container.Cluster name cannot exceed 40 characters. MageLift must derive the cluster name from Magento project+environment (naming.ClusterName) and truncate to 40 — never prefix with the GCP project ID (long project IDs + Magento prefix overflow). runtime.Args needs MagentoProject+Environment separate from GCP Project.
