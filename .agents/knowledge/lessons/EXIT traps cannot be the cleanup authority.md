---
type: lesson
title: EXIT traps cannot be the cleanup authority
description: Record owned resources in a restartable ledger before provider create; traps disappear on kill, crash, or sleep.
tags: [cleanup, acceptance, scaleway, lifecycle]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-13
---

# Rule

Write a durable ownership ledger before the provider create call. An EXIT trap is last-mile cleanup for a still-running process. It is not the lifecycle: a kill, crash, laptop sleep, or interrupted wait leaves the trap unexecuted while paid resources remain.

# Why

The Scaleway Managed Database cell `live-rdb-20260813g` created source instance `2e0285e9-8109-4bf0-bc83-93cab9c230b4` and snapshot `b2f67c25-a443-4605-8508-92440d37a395`. The wrapper process disappeared before the trap finished. Direct inventory still showed both objects until a later marker-and-ID cleanup deleted the snapshot first, then the instance.

# MageLift

Use `magelift cleanup claim` (or the acceptance helper) before mutation, `magelift cleanup record` after the provider identity exists, and `magelift cleanup reconcile --dir .magelift/cleanup --yes` to finish interrupted runs. Reconcile unions the ledger with a direct owning-service inventory, deletes only exact owned identities in rank order, and treats tombstones as pending rather than success.

Live runs: do not interrupt. Two 2026-09-15 interrupts (Scaleway 8 minutes in, OVH 25/30 minutes in) both orphaned billable resources: the EXIT-trap destroy races workdir cleanup and loses, so manual ordered teardown was required both times. Launch live EU runs with a 60-minute window and do not touch them. If interrupted anyway, sweep immediately via owning-service inventory.
