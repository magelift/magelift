---
type: lesson
title: RollingUpdate keeps Ready Magento during pause inject
description: kubectl set-image to pause leaves the previous Ready Magento pod until the new ReplicaSet is Ready; kube health stays 1/1. Recreate + delete the old pods so Ready drops.
tags: [gke, failed-deployment, kubectl, rolling-update]
status: stable
generated:
  by: cursor-grok
  at: '2026-08-19'
---

`kubectl set image` on `gcap25-preview-app-web` with `registry.k8s.io/pause` succeeded. The new ReplicaSet CrashLoopBackOff at 1/2. RollingUpdate kept the 11h Magento pod 2/2 Ready, so `magelift health --mode runtime` stayed `1 ready of 1 desired`. After 180s the drill printed `failed-deployment apply failed but runtime is healthy`. Restore via `magelift deploy --digest` of the current image did not revert the out-of-band mutation (Pulumi saw 41 same, k8s provider only).

Inject with Recreate strategy, then delete pods labeled `app=<web-deployment>`. Restore with `kubectl set image` back to the current digest (pair of the inject), not Pulumi, unless a refresh has recorded the drift.
