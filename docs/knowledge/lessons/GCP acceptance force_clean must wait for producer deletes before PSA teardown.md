---
type: lesson
title: GCP acceptance force_clean must wait for producer deletes before PSA teardown
description: force_clean_orphans issues async deletes for GKE, Cloud SQL, Memorystore, and SCP.
tags:
- gcp
- acceptance
- psa
- force_clean
generated:
  by: cursor/wsl
  at: '2026-07-19T20:33:56.947975000+00:00'
---

force_clean_orphans issues async deletes for GKE, Cloud SQL, Memorystore, and SCP. assert_clean must not run until those producers are gone — otherwise VPC/PSA peering teardown races and leaves mlacc-*-net, Valkey, and SCP orphans while SQL is still RUNNING. Poll (default 20m, 30s interval) and re-issue deletes until producers clear, then delete peering/NAT/subnets/VPC. On EXIT trap, prefer waiting over failing assert_clean early. Optional: refresh GOOGLE_OAUTH_ACCESS_TOKEN before long deploy (gcloud tokens expire mid-run).
