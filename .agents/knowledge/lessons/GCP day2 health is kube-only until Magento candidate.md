---
type: lesson
title: GCP day2 health is kube-only until Magento candidate
description: Standard HA LoadBalancer HTTP 500 before deploy:candidate must not fail day2:health; Magento HTTP 200 is the post-candidate gate.
tags: [gcp, gke, ha, health, magento, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-19
---

# GCP day2 health is kube-only until Magento candidate

The HA catalog runs `day2:health` after create-once and before
`migrate:dump` / `deploy:candidate`. Autopilot often omits Magento HTTP
because `applicationURL` is empty. GKE Standard publishes a LoadBalancer IP,
so `CheckRuntime` probes Magento and records `runtime.web` unhealthy on HTTP
500 from infra-only PHP.

`gcha31` kube 3/3 stayed healthy and still failed `day2:health` after the
600s retry. That is catalog order, not a Magento image bug.

`day2:health` now passes on `runtime.kube.deployment` healthy. Magento HTTP
200 remains required after `deploy:candidate`.
