---
type: lesson
title: GKE Autopilot Cloud Logging components are SYSTEM_COMPONENTS and WORKLOADS
description: Autopilot create-auto enables SYSTEM_COMPONENTS and WORKLOADS, not the obsolete SYSTEM and WORKLOAD enum values.
tags: [gcp, gke, autopilot, logging, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

# GKE Autopilot Cloud Logging components are SYSTEM_COMPONENTS and WORKLOADS

Cell `20260818a` created Autopilot cluster `ml-gke-nobs-20260818a` in
`europe-west1`. `loggingConfig.componentConfig.enableComponents` was
`SYSTEM_COMPONENTS` and `WORKLOADS`. A harness check for `SYSTEM` and
`WORKLOAD` failed immediately, then the EXIT trap deleted the cluster.

Require the current GKE enum names. Do not reuse `a`.
