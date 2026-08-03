---
type: lesson
title: ECS Exec requires service and task-role wiring
description: The CLI ECS Exec path is only valid when the ECS service sets enableExecuteCommand=true and
  the application task role grants ssmmessages CreateControlChannel/CreateDataChannel/OpenControlChannel/Op...
tags:
- aws
- ecs
- ecs-exec
- iam
- security
generated:
  at: '2026-07-24'
---

The CLI ECS Exec path is only valid when the ECS service sets enableExecuteCommand=true and the application task role grants ssmmessages CreateControlChannel/CreateDataChannel/OpenControlChannel/OpenDataChannel. The execution role policy alone is insufficient. Pulumi mocks now assert both settings.
