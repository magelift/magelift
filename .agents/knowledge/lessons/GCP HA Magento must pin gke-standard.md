---
type: lesson
title: GCP HA Magento must pin gke-standard
description: Inherited MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-autopilot defeats the HA Standard override; 3-node OpenSearch then exits 78.
tags: [gcp, gke, ha, opensearch, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-19
---

# GCP HA Magento must pin gke-standard

`scripts/gcp-acceptance-local.sh` only sets HA to `gke-standard` when
`MAGELIFT_GCP_ACCEPTANCE_RUNTIME` is unset. A leftover
`MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-autopilot` from a preview KEEP (for
example `gcap27`) keeps YAML on Autopilot.

Three-node OpenSearch then CrashLoopBackOff with exit 78 (`vm.max_map_count`
too low). Autopilot cannot set that sysctl. Magento seed-probe never starts.

HA Magento must export `MAGELIFT_GCP_ACCEPTANCE_RUNTIME=gke-standard`
explicitly. The harness now fails closed if HA still resolves to
`gke-autopilot`. Do not treat Autopilot OpenSearch 3-replica failure as a
Magento bug.
