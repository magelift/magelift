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
  by: maintainer
  at: '2026-07-20T00:41:36.832368000'
---

After Cloud SQL delete, servicenetworking Connection delete often fails with FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION or an internal code 13 while a producer is still listed or the control plane is catching up. gcloud services vpc-peerings delete --force is gone. Wait for producers (SQL/Memorystore/GKE/SCP), soak ~3m, then Compute networks.removePeering (REST) before deleting the PSA GlobalAddress and VPC. MageLift: Cache DependsOn Database so Valkey finishes before PSA teardown; the acceptance destroy path nudges an async peering delete once producers clear, then force_clean repeats the exact cleanup. The m246s8 GCP standard pass still hit this race after a successful candidate and ended clean only after the soak/retry path. Connection Delete timeout 45m remains a last resort. Relates to GCP acceptance force_clean lesson.
