---
type: lesson
title: MageLift deployment lifecycle hooks 2026-07-18
description: The provider-neutral deploy orchestrator now accepts NewWithHooks.
tags:
- deploy
- hooks
- sdk
- extension
status: stable
generated:
  at: '2026-07-24'
---

The provider-neutral deploy orchestrator now accepts NewWithHooks. SDK LifecycleHook includes Validate and Run; descriptors are restricted to stable deploy/post-deploy target IDs, validated before lock acquisition, topologically ordered by dependencies, bounded by timeout and idempotent retry policy, and support before/after/replace/disable. Existing New remains hook-free for AWS CLI compatibility.
