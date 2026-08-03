---
type: lesson
title: MageLift runtime rollback local verification 2026-07-18
description: After the runtime capability identity split, rollback acknowledgement wiring, local OpenSearch
  and RabbitMQ Compose profiles, and bootstrap KMS ownership guard, GOTMPDIR=/home/alex/go/tmp make veri...
tags:
- verification
- floci
- race
- composer
- pulumi
status: stable
generated:
  at: '2026-07-24'
---

After the runtime capability identity split, rollback acknowledgement wiring, local OpenSearch and RabbitMQ Compose profiles, and bootstrap KMS ownership guard, GOTMPDIR=/home/alex/go/tmp make verify passed. It ran generated config and CLI drift checks, gofmt, vet, race tests across all packages, license scan, Composer validation and audit, PHPStan, Psalm, PHPUnit, MkDocs strict, and actionlint. make floci-test passed the account-free bootstrap, state, lock, and ECS runtime health suite using Floci 1.5.33.
