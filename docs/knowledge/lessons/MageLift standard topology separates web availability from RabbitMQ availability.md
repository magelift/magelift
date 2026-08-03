---
type: lesson
title: MageLift standard topology separates web availability from RabbitMQ availability
description: The standard preset keeps the application availability minimum at two zones, while its RabbitMQ
  capability requires three zones for a quorum cluster.
tags:
- topology
- rabbitmq
- availability
status: stable
generated:
  at: '2026-07-24'
---

The standard preset keeps the application availability minimum at two zones, while its RabbitMQ capability requires three zones for a quorum cluster. Target synthesis must provision or import a third private queue subnet without silently weakening the queue requirement.
