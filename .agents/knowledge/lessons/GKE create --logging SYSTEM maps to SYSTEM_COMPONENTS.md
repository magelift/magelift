---
type: lesson
title: GKE create --logging SYSTEM maps to SYSTEM_COMPONENTS
description: gcloud container clusters create --logging=SYSTEM,WORKLOAD stores enableComponents SYSTEM_COMPONENTS and WORKLOADS.
tags: [gcp, gke, logging, acceptance]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-18
---

# GKE create --logging SYSTEM maps to SYSTEM_COMPONENTS

The Standard create flag set is `--logging=SYSTEM,WORKLOAD`. Cell
`20260818d` then described `loggingConfig.componentConfig.enableComponents` as
`SYSTEM_COMPONENTS` and `WORKLOADS`, the same names Autopilot `create-auto`
returns.

Assert both enum families. Do not treat CLI flag names and describe JSON as
the same strings.
