---
type: lesson
title: Log-based alert filters must restrict resource.type
description: Cloud Monitoring requires resource.type on log-metric alerts, matching the log resource (k8s_container for GKE workload logs), not global.
tags: [gcp, monitoring, logging, alerting, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

# Log-based alert filters must restrict resource.type

Cell `20260818h` omitted `resource.type` and Monitoring rejected the policy.
Cell `20260818i` used `resource.type="global"` and Monitoring rejected it as
an invalid metric/resource combination.

User-defined log-based metrics inherit the log's monitored resource. GKE
container stdout is `k8s_container`. Put `cluster_name` in the metric log
filter and `resource.type="k8s_container"` on the alert condition.

Do not reuse `h` or `i`.
