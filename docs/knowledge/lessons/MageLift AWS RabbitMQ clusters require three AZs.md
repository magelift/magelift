---
type: lesson
title: MageLift AWS RabbitMQ clusters require three AZs
description: Amazon MQ for RabbitMQ cluster deployments use three broker nodes across three Availability
  Zones.
tags:
- aws
- rabbitmq
- queue
- topology
status: stable
generated:
  at: '2026-07-24'
---

Amazon MQ for RabbitMQ cluster deployments use three broker nodes across three Availability Zones. Standard and high-availability queue components therefore require three private queue subnets. Preview remains database-backed.
