---
type: lesson
title: GCP Autopilot destroy after one hour uses DELETE_UNREACHABLE
description: Stack kubeconfig is a static OAuth token from apply. After ~1h Pulumi k8s destroy fails credentials while gcloud kubectl still works. Set PULUMI_K8S_DELETE_UNREACHABLE=true so GKE cluster delete via GCP API wipes workloads.
tags: [gcp, gke, pulumi, destroy, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

`gcap24` KEEP destroy failed in 2s with Pulumi Kubernetes
`configured Kubernetes cluster is unreachable` / `the server has asked for
the client to provide credentials` on Ingress `gcap24-preview-edge-ingress`.
`gcloud container clusters describe` still showed `RUNNING`. `kubectl` with
`gcloud container clusters get-credentials` listed Ingress, web, cron, and
search. `gke-gcloud-auth-plugin` is not on PATH and is not the contract;
`generateKubeconfig` bakes a static OAuth token into stack state at apply.

That token lasts about an hour. Catalog apply at 18:22 UTC, destroy at 20:36
UTC. Refreshing `KUBECONFIG` / `GOOGLE_OAUTH_ACCESS_TOKEN` in the Magelift
process does not replace the token inside the Kubernetes provider resource.

When the GKE cluster still exists and only the baked kubeconfig is stale, set
`PULUMI_K8S_DELETE_UNREACHABLE=true` for `magelift destroy`. Pulumi drops
Kubernetes objects from state without calling the API; the GCP GKE cluster
delete still runs and removes the workloads. Do not use the flag to skip a
cluster that is already gone unless that is the actual inventory.

See [GKE Pulumi k8s provider must not require gke-gcloud-auth-plugin](GKE%20Pulumi%20k8s%20provider%20must%20not%20require%20gke-gcloud-auth-plugin.md).
