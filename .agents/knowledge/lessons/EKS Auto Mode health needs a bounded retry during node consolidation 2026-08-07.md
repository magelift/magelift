---
type: lesson
title: EKS Auto Mode health needs a bounded retry during node consolidation
description: Auto Mode can evict a healthy web pod while a warm cell is being checked, so a single immediate readiness probe can produce a false failure.
tags: [aws, eks, kubernetes, acceptance, reliability]
status: stable
generated:
  by: codex
  at: 2026-08-07
---

# Observation

During the EKS search-disabled warm cell, Auto Mode consolidated an
underutilized node and evicted the ready web pod at the same time as the
infra-only update. The new pod was healthy shortly afterward, but one direct
runtime health command observed zero ready replicas.

# Rule

Keep infrastructure-only updates separate from certification assertions. After
the update, wait for the Deployment's desired replicas with a bounded retry
window. Preserve the first failure and the replacement pod events in the cell
log so a real rollout failure is still visible.

# MageLift

The AWS acceptance harness already uses a bounded runtime-health retry after
create-once and cell updates. The EKS live check passed on the retry after the
replacement pod became ready. The same rule applies to GKE, Scaleway, and OVH
managed Kubernetes runtimes.
