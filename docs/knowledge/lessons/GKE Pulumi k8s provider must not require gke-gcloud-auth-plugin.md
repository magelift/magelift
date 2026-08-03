---
type: lesson
title: GKE Pulumi k8s provider must not require gke-gcloud-auth-plugin
description: Pulumi kubernetes provider against GKE Autopilot failed with gke-gcloud-auth-plugin not found
  when kubeconfig used exec auth.
tags:
- gcp
- gke
- pulumi
- kubernetes
generated:
  by: cursor/wsl
  at: '1784492481001398'
---

Pulumi kubernetes provider against GKE Autopilot failed with gke-gcloud-auth-plugin not found when kubeconfig used exec auth. Use organizations.GetClientConfigOutput access token + cluster endpoint + MasterAuth CA in an inline kubeconfig (token auth). Acceptance hosts/agents must not depend on the gcloud auth plugin binary.
