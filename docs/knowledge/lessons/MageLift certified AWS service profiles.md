---
type: lesson
title: MageLift certified AWS service profiles
description: V1 AWS runtime target is ECS Fargate.
tags:
- magelift
- aws
- ecs
- services
- v1
status: stable
generated:
  at: '2026-07-24'
---

V1 AWS runtime target is ECS Fargate. Production golden database is provisioned Multi-AZ Aurora MySQL; later adapters add RDS MariaDB/MySQL with strict Adobe compatibility gating. Queue profiles: Magento DB queues for cheap preview environments and three-node Amazon MQ RabbitMQ for production; do not claim Amazon MQ Artemis support. Media uses Magento S3 remote storage and the golden path has no EFS. Search uses provisioned Multi-AZ OpenSearch domain in production and scale-to-zero OpenSearch Serverless for disposable environments. Ship preview, standard, and high-availability presets with bounded overrides and cost estimates. Standard integrated and headless modes share the backend runtime and differ at edge/routing policy.
