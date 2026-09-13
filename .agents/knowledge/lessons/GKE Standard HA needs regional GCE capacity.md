---
type: lesson
title: GKE Standard HA needs regional GCE capacity
description: europe-west1 and europe-west4 can refuse a GKE Standard HA cluster with not-enough-resources; do not immediately retry the same region.
tags: [gcp, gke, ha, capacity, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-19
---

# GKE Standard HA needs regional GCE capacity

`gcha30` create-once in `europe-west1` failed after 247s:

`Google Compute Engine does not have enough resources available to fulfill request: europe-west1.`

`gcha35` create-once in `europe-west4` failed the same way at 106s of GKE
cluster create (31 resources created, stack failed at 51m44s after Valkey
finished). Autopilot Magento (`gcap27`) and earlier Standard HA create-once
(`gcha31`/`gcha32`) had succeeded in `europe-west4` the previous night.

The miss is node-pool GCE stock, not Magento. Do not immediately retry the
same region. Pick a new prefix and a different region.
