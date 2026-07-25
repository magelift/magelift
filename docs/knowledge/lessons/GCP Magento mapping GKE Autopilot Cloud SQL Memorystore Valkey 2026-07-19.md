---
type: lesson
title: GCP Magento mapping GKE Autopilot Cloud SQL Memorystore Valkey 2026-07-19
description: 'Experimental GCP target gcp.gke-autopilot maps: GKE Autopilot for web/cron/deploy/queue
  workloads; Cloud SQL MySQL private IP + Secret Manager password; Memorystore for Valkey (CLUSTER_DISABLED,
  VA...'
tags:
- gcp
- gke
- cloudsql
- valkey
status: stable
generated:
  by: cursor/wsl
  at: '1784487846611378'
---

Experimental GCP target gcp.gke-autopilot maps: GKE Autopilot for web/cron/deploy/queue workloads; Cloud SQL MySQL private IP + Secret Manager password; Memorystore for Valkey (CLUSTER_DISABLED, VALKEY_8_0) via DesiredAutoCreatedEndpoints; Cloud NAT for Autopilot egress; LB Service for applicationURL. Defer search/RabbitMQ/CDN/WIF/GCS DIY bootstrap. Project digital-lab-341608 had compute+artifactregistry enabled; enable container/sqladmin/memorystore/secretmanager/servicenetworking before real preview.
