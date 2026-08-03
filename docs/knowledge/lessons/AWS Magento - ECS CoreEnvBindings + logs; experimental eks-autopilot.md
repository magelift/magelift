---
type: lesson
title: 'AWS Magento: ECS CoreEnvBindings + logs; experimental eks-autopilot'
description: ECS Magento env goes through platform.CoreEnvBindings; queue/search/media stay adapter-local.
tags:
- aws
- ecs
- eks
- ports
generated:
  by: cursor/wsl
  at: '2026-07-19T23:54:10.313848000'
---

ECS Magento env goes through platform.CoreEnvBindings; queue/search/media stay adapter-local. Observability NewLogGroups runs before ECS runtime so awslogs has CW groups; containers set logConfiguration. Experimental aws/eks-autopilot is eks + eksops (reuse network/db/cache; stub Ops/Observe; reuse Bootstrap/State/Secrets). catalog.eks vs catalog.fargate validated by runtime. DIY stack name project-env-aws-eks-autopilot. Cost/ci gated to ecs-fargate; Composer secrets via options.newComposerSecrets factory.
