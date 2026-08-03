---
type: lesson
title: Keep Floci over Ministack for MageLift offline tests 2026-07-19
description: 'Decision 2026-07-19: retain Floci as the sole offline AWS emulator.'
tags:
- floci
- ministack
- testing
- decision
status: deprecated
generated:
  at: '2026-07-24'
---

Decision 2026-07-19: retain Floci as the sole offline AWS emulator. Current Floci claims 68 AWS services including Valkey-backed ElastiCache, real Amazon MQ RabbitMQ containers, Aurora/MySQL containers, OpenSearch real dataplane, ECS/ELB/CloudFront/WAF/KMS — closer to MageLift golden path than Ministack (Redis not Valkey; MQ management-plane only). Floci also ships floci-az and floci-gcp for future multi-cloud. Known gap on pinned 1.5.33: IAM GetOpenIDConnectProvider UnsupportedOperation; retest OIDC on newer Floci before expanding suite. Ministack remains a rejected alternative unless Floci OIDC stays broken after upgrade. Real AWS matrix still required for certification.

# Related

* Supersedes: Projects/magelift/Findings/Evaluate Ministack as Floci complement 2026-07-19
