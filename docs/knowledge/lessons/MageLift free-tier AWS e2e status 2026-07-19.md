---
type: lesson
title: MageLift free-tier AWS e2e status 2026-07-19
description: 'Escape hatches shipped in working tree (uncommitted): natMode=fck-nat, databaseEngine=rds-mysql,
  searchMode=disabled.'
tags:
- e2e
- status
- free-tier
status: stable
generated:
  at: '2026-07-24'
---

Escape hatches shipped in working tree (uncommitted): natMode=fck-nat, databaseEngine=rds-mysql, searchMode=disabled. Fixed: ctx.Export stack outputs; RDS secret host/port/dbname via env; nginx /var/log|/var/cache tmpfs; varnish malloc/tmpfs scaled to task memory. e2e4: infra created (~102 resources), migration task exit 0, cron steady, web TaskFailedToStart (web exit 1 / varnish 137) then stabilize timeout; destroy still cleaning vpc+logs. Full green path (DEPLOY_OK+HEALTH_OK+E2E_CLEAN_OK) not yet achieved. Account must be verified clean before next deploy.
