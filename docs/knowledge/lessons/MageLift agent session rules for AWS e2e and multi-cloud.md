---
type: lesson
title: MageLift agent session rules for AWS e2e and multi-cloud
description: 'Always: (1) Query Memgraph for magelift lessons before AWS work.'
tags:
- session
- aws
- e2e
- architecture
- pulumi
status: stable
generated:
  at: '2026-07-24'
---

Always: (1) Query Memgraph for magelift lessons before AWS work. (2) Use repo .agents/skills/pulumi-* before changing Pulumi Automation/inline programs. (3) Prefer official AWS docs (docs.aws.amazon.com) over memory for RDS secrets, ECS, CloudFront, Fargate limits. (4) Real AWS e2e must leave zero leftovers: EXIT trap destroy + assert_clean vpc/rds/cache/alb/logs/sg. (5) Never kill mid-deploy; clear DIY locks + pending_operations only after confirming AWS empty. (6) Program must ctx.Export Component.Outputs — RegisterResourceOutputs alone is invisible to stack.Outputs. (7) RDS ManageMasterUserPassword JSON has only username/password; host/port/dbname are env from WriterEndpoint. (8) Keep provider-specific code under internal/cloud/<provider>; shared deploy/health/config contracts stay provider-agnostic. (9) Prefer small escape-hatch fields over silent shape swaps. (10) Humanize user-facing prose; keep code/comments why-not-what.
