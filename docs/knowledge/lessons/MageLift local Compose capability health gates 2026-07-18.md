---
type: lesson
title: MageLift local Compose capability health gates 2026-07-18
description: The generated local Compose app must wait for healthy database, Valkey, OpenSearch, and RabbitMQ
  services.
tags:
- magelift
- local-development
- compose
- healthcheck
generated:
  at: '2026-07-24'
---

The generated local Compose app must wait for healthy database, Valkey, OpenSearch, and RabbitMQ services. Valkey now has a valkey-cli ping healthcheck; app depends_on uses service_healthy for all four capability services. This prevents first-run Magento startup races and is covered by internal/localdev tests.
