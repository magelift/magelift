---
type: lesson
title: Magento AMQP config uses user key
description: Magento reads queue/amqp/user, so platform environment names must be translated to that config key before migration or topology commands run.
tags:
- magento
- amqp
- rabbitmq
- configuration
- gcp
- acceptance
status: stable
generated:
  by: codex/desktop
  at: '2026-08-05'
---

The first GCP RabbitMQ candidate reached Magento migration but failed in
`FactoryOptions::setUsername()` because MageLift emitted `queue/amqp/username`.
Adobe's Magento code and ece-tools use `queue/amqp/user`; the platform-level
environment variable may remain `MAGENTO_DC_QUEUE__AMQP__USERNAME`, but the
runtime `env.php` template must map it to the `user` key.

Keep a template contract test and a migration Job test for this translation.
Apply the same check to every provider that runs Magento workloads because the
failure is in the shared application contract, not in GCP networking.
