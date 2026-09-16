---
type: lesson
title: Greenfield deploy must bootstrap infra before ECS candidate
description: magelift deploy RegisterCandidate needs clusterName outputs.
tags:
- aws
- deploy
- pulumi
generated:
  at: '2026-07-24'
---

magelift deploy RegisterCandidate needs clusterName outputs. Empty stacks must Update first then register (fixed 56f367c). First deploy chicken-egg otherwise. Live stacks go the other way: use deploy --infra-only for spec updates. A full deploy on a live stack tries to re-create the migrate Job and fails Unauthorized; infra-only applies cleanly (proven: web and cron Deployment updates only).
