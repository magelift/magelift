---
type: lesson
title: GCP PSA teardown needs removePeering after Cloud SQL soft-delete lag
description: After Cloud SQL delete, servicenetworking Connection delete often fails with FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION
  (producer still listed briefly).
tags:
- gcp
- psa
- destroy
- acceptance
generated:
  by: cursor/wsl
  at: '2026-07-20T00:41:36.832368000'
---

After Cloud SQL delete, servicenetworking Connection delete often fails with FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION (producer still listed briefly). gcloud services vpc-peerings delete --force is gone. Wait for producers (SQL/Memorystore/GKE/SCP), soak ~3m, then Compute networks.removePeering (REST) before deleting the PSA GlobalAddress and VPC. MageLift: Cache DependsOn Database so Valkey finishes before PSA teardown; force_clean uses soak + removePeering; Connection Delete timeout 45m is belt-and-suspenders. Relates to GCP acceptance force_clean lesson.
