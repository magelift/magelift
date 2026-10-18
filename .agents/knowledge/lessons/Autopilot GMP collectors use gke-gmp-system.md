---
type: lesson
title: Autopilot GMP collectors use gke-gmp-system
description: GKE Autopilot managed Prometheus collectors are a DaemonSet named collector in gke-gmp-system, not gmp-system.
tags: [gcp, gke, autopilot, prometheus, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

# Autopilot GMP collectors use gke-gmp-system

Cell `20260818e` created Autopilot `ml-gke-nobs-20260818e` with
`managedPrometheusConfig.enabled=true`, then failed because the harness looked
only at `gmp-system/collector`. Google documents Autopilot collectors in
`gke-gmp-system`; Standard keeps `gmp-system`.

Wait for `collector` in either namespace. Do not reuse `e`.
