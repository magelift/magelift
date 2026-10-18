---
type: lesson
title: GA gcloud has no monitoring time-series command
description: List Kubernetes uptime series through Monitoring API v3 projects.timeSeries.list; gcloud monitoring time-series is not a GA command.
tags: [gcp, monitoring, gcloud, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

# GA gcloud has no monitoring time-series command

Cell `20260818b` proved Autopilot workload logs and container audit events, then
timed out with `metrics=0`. The probe invoked `gcloud monitoring time-series
list`, which GA gcloud 579 rejects (`Invalid choice: 'time-series'`). Stderr was
discarded, so the loop looked like missing GKE metrics.

Use the documented REST method
`GET https://monitoring.googleapis.com/v3/projects/{project}/timeSeries` with
`filter`, `interval.startTime`, `interval.endTime`, and `view`, authenticated
by `gcloud auth print-access-token`. That is the same owning-API pattern as
metric-descriptor cleanup in the control-plane observability cell. Do not
reuse `b`.
