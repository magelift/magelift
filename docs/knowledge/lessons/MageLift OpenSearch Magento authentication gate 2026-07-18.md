---
type: lesson
title: MageLift OpenSearch Magento authentication gate 2026-07-18
description: The AWS OpenSearch component uses IAM task-role access and therefore needs SigV4 requests.
tags:
- aws
- opensearch
- magento
- security
- release-gate
status: stable
generated:
  at: '2026-07-24'
---

The AWS OpenSearch component uses IAM task-role access and therefore needs SigV4 requests. MageLift now adds the pinned public.ecr.aws/aws-observability/aws-sigv4-proxy:1.11.1 sidecar to ECS web, deploy, cron, and queue task definitions, routes Magento OpenSearch settings to 127.0.0.1:8081, and configures the proxy for es or aoss based on the endpoint. The release gate remains active until a real AWS matrix proves index creation, catalog indexing, queries, reconnects, proxy failure recovery, and least-privilege behavior. OpenSearch Serverless and managed domains are not certified by Floci or Pulumi mocks.
