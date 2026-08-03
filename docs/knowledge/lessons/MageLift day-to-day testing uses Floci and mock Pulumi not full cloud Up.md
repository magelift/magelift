---
type: lesson
title: MageLift day-to-day testing uses Floci and mock Pulumi not full cloud Up
description: Full AWS/GCP Magento acceptance (100+ resources) routinely takes 30–60+ minutes because of
  RDS/Cloud SQL, ElastiCache/Memorystore, ALB/CloudFront or GKE Autopilot, plus Magento migrate+ALB stabiliz...
tags:
- testing
- floci
- acceptance
- performance
generated:
  by: cursor/wsl
  at: '2026-07-19T22:53:27.847245000'
---

Full AWS/GCP Magento acceptance (100+ resources) routinely takes 30–60+ minutes because of RDS/Cloud SQL, ElastiCache/Memorystore, ALB/CloudFront or GKE Autopilot, plus Magento migrate+ALB stabilize — not because Pulumi/Go is slow. Day-to-day loop: Floci + unit tests + mock Pulumi graphs. Real cloud: keep one preview stack only while iterating (KEEP=true), destroy same day, one focused path, EXIT trap + assert_clean. Reserve full create/destroy for certification gates.
