---
type: lesson
title: MageLift provider-neutral capability requirement gate
description: sdk/v1 now defines stable logical runtime capability IDs and ValidateCapabilityRequirements,
  which checks stable IDs, duplicates, and unsupported requirements without contacting a cloud provider.
tags:
- sdk
- capabilities
- aws
- validation
status: stable
generated:
  at: '2026-07-24'
---

sdk/v1 now defines stable logical runtime capability IDs and ValidateCapabilityRequirements, which checks stable IDs, duplicates, and unsupported requirements without contacting a cloud provider. The AWS stack advertises database.mysql, cache.valkey, search.opensearch, object-storage.s3, edge.cloudfront, observability.cloudwatch, plus queue.database for preview or queue.rabbitmq for standard/HA. Stack Spec validation rejects a declared artifact requirement that the selected preset cannot supply before Pulumi registration. Empty requirements remain valid until config planning ingests a mandatory artifact manifest.
