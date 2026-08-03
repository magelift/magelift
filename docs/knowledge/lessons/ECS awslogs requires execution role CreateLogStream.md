---
type: lesson
title: ECS awslogs requires execution role CreateLogStream
description: Wiring container logConfiguration to /magelift/<project>/<env>/{web,deploy,cron} is insufficient
  alone.
tags:
- aws
- ecs
- logs
- iam
generated:
  by: cursor/wsl
  at: '2026-07-20T00:40:08.715468000'
---

Wiring container logConfiguration to /magelift/<project>/<env>/{web,deploy,cron} is insufficient alone. The ECS task execution role must allow logs:CreateLogStream and logs:PutLogEvents on those log-group ARNs (and log-stream:*). Without it, candidate migrate/deploy tasks fail ResourceInitializationError AccessDeniedException on CreateLogStream before Magento runs. Fix: attachExecutionLogPolicy in aws/runtime when LogGroupPrefix is set.
