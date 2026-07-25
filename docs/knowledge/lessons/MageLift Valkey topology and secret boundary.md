---
type: lesson
title: MageLift Valkey topology and secret boundary
description: The AWS Valkey component uses one replication group for preview with at most one replica,
  while standard and high-availability use separate cache and session groups so cache eviction cannot
  invalid...
tags:
- aws
- valkey
- elasticache
- secrets
- topology
status: stable
generated:
  at: '2026-07-24'
---

The AWS Valkey component uses one replication group for preview with at most one replica, while standard and high-availability use separate cache and session groups so cache eviction cannot invalidate sessions. Capacity, engine version, and node type remain caller and benchmark supplied. Authentication inputs are Secrets Manager ARNs only; each referenced AWSCURRENT SecretString must contain the raw ElastiCache auth token, not JSON. Pulumi marks the resolved value secret before it reaches replication group inputs.
