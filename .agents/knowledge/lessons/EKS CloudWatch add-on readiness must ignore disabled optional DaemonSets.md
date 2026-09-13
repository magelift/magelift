---
type: lesson
title: EKS CloudWatch add-on readiness must ignore disabled optional DaemonSets
description: EKS CloudWatch collection exposes optional Windows DaemonSets with desired zero, so readiness proof must select active Linux collectors and verify delivery through owning APIs.
tags: [aws, eks, cloudwatch, observability, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-17
sources:
  - id: cloudwatch-observability-addon
    resource: https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/install-CloudWatch-Observability-EKS-addon.html
    title: AWS CloudWatch Observability EKS add-on
  - id: container-insights-logs
    resource: https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Container-Insights-EKS-logs.html
    title: AWS Container Insights EKS logs
  - id: container-insights-metrics
    resource: https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Container-Insights-EKS-metrics.html
    title: AWS Container Insights EKS metrics
---

# EKS CloudWatch add-on readiness must ignore disabled optional DaemonSets

The `amazon-cloudwatch-observability` EKS add-on can install optional
Windows collectors even when a cluster has no Windows capacity. Those
DaemonSets legitimately report desired and ready counts of zero. A readiness
predicate that requires every CloudWatch DaemonSet to be ready therefore
rejects a healthy Linux-only cluster.

For a Linux self-managed cluster, select the active Linux collectors by exact
name (`cloudwatch-agent` and `fluent-bit`), then require each selected
DaemonSet to have a positive desired count and `ready == desired`. The
DaemonSet check is only installation/readiness evidence. Delivery must be
proved through the owning CloudWatch APIs: an exact application-log marker,
at least one relevant Container Insights metric data point, and EKS control
plane audit streams/events when audit logging is in scope.

Keep the proof bounded. A successful add-on, collector, log, metric, and
audit read does not establish redaction, alert delivery, SLO delivery,
custom OpenTelemetry behavior, Windows support, or failure-domain recovery.
Retained-stack resumes also need an explicit resume opt-in and an exact
project/run ownership check; a checkpoint alone is not sufficient authority to
skip creation or cleanup.
