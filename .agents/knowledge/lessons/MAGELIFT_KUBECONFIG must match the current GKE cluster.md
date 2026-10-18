---
type: lesson
title: MAGELIFT_KUBECONFIG must match the current GKE cluster
description: Inherited gcloud kubeconfig from a previous KEEP can reuse the GKE public IP with a stale CA and fail day2:logs as x509 unknown authority.
tags: [gcp, gke, kubeconfig, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-19
---

# MAGELIFT_KUBECONFIG must match the current GKE cluster

`magelift logs` / exec prefer `MAGELIFT_KUBECONFIG` over the Pulumi stack
kubeconfig so a KEEP older than one hour still works. That file is bound to
one cluster’s CA.

A leftover path from `gcap27` or `gcha28` is not safe on the next cell. GCP
can assign the same GKE public endpoint IP. TLS then fails as `x509:
certificate signed by unknown authority` instead of connection refused.
`gcha29` failed `day2:logs` that way after a clean GKE Standard create-once.

The GCP harness now drops inherited `MAGELIFT_KUBECONFIG` and refreshes it
from `gcloud container clusters get-credentials` after create-once. The kube
client rejects an override that does not contain the stack `clusterName`.
