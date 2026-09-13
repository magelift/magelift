---
type: lesson
title: Scaleway Kapsule multi-AZ needs one pool per zone
description: A Scaleway Kapsule multi-AZ profile must translate each configured zone into a worker pool and enforce workload spreading before it can claim zone resilience.
tags: [scaleway, kapsule, kubernetes, multi-az, high-availability]
status: stable
generated:
  by: codex
  at: 2026-08-11
sources:
  - id: scaleway-kapsule-multi-az
    resource: https://www.scaleway.com/en/docs/kubernetes/reference-content/multi-az-clusters/
    title: Scaleway Kapsule multi-AZ clusters
  - id: scaleway-kapsule-multi-az-tutorial
    resource: https://www.scaleway.com/en/docs/tutorials/k8s-kapsule-multi-az/
    title: Deploying a multi-AZ Kubernetes cluster with Kapsule
---

# Mechanism

Scaleway Kapsule's `zone` is a property of a node pool. A multi-AZ cluster is
therefore assembled from multiple pools in the same cluster and Private Network,
with one pool associated with each selected zone. The Kapsule API does not turn a
single pool into a multi-AZ placement merely because the cluster has multiple
eligible zones.

MageLift's Scaleway runtime maps `target.scaleway.zones` to one pool per zone and
distributes `nodeCount` deterministically, requiring at least one node per zone.
Replicated web and queue workloads receive a strict
`topology.kubernetes.io/zone` spread constraint when their replica count can cover
the configured zones. The planner rejects standard and high-availability profiles
whose zone list is smaller than the named topology minimum before Pulumi mutation.

# Failure modes

* A single pool with several replicas can leave every node in one zone, so a
  successful cluster update does not prove zone resilience.
* `ScheduleAnyway` permits all replicas to land in one zone; use
  `DoNotSchedule` for workloads whose replica count can satisfy the spread.
* Scaleway documents zone-local persistent volumes, control-plane reachability,
  and a single Public Gateway as multi-AZ limitations. A multi-AZ worker graph
  does not by itself prove control-plane, storage, gateway, or application HA.
* If the requested node count is lower than the number of zones, at least one
  zone would be empty; reject the plan rather than quietly weakening it.

# Related

