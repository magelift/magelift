---
type: lesson
title: MageLift rollback requires forward-only acknowledgement 2026-07-18
description: Rollback is a forward deployment of an older signed digest.
tags:
- deployments
- rollback
- database
status: stable
generated:
  at: '2026-07-24'
---

Rollback is a forward deployment of an older signed digest. The CLI requires --ack-forward-only before inspecting or mutating the release journal, passes Rollback and AcknowledgeForwardOnlyDB into the deployment orchestrator, and warns that migrations are not reversed. Documentation directs teams to expand/contract migration design.
