---
type: lesson
title: MageLift preset topology remains provider neutral
description: Preset intent is modeled in sdk/v1 as desired workloads, capability requirements, failure-domain
  availability, durability, isolation, PITR, scale-to-zero permission, and preview TTL/budget guards.
tags:
- architecture
- topology
- aws
- eks
- multi-cloud
- presets
generated:
  at: '2026-07-24'
---

Preset intent is modeled in sdk/v1 as desired workloads, capability requirements, failure-domain availability, durability, isolation, PITR, scale-to-zero permission, and preview TTL/budget guards. It deliberately contains no AWS product names, instance types, task sizes, or guessed capacity. internal/topology constructs deterministic preview, standard, and high-availability topologies. Provider targets validate whether they can satisfy the same model, allowing future EKS and non-AWS targets without changing project YAML.
