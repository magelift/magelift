---
type: lesson
title: MageLift final account-free verification 2026-07-18
description: After adding encryption-key secret references and ECS JSON selectors, make verify passed
  with Go race tests, vet, Composer validation and audit, Psalm, PHPUnit 96 tests and 229 assertions,
  strict M...
tags:
- verification
- floci
- release
generated:
  at: '2026-07-24'
---

After adding encryption-key secret references and ECS JSON selectors, make verify passed with Go race tests, vet, Composer validation and audit, Psalm, PHPUnit 96 tests and 229 assertions, strict MkDocs, and actionlint. make image-test, make frankenphp-image-test, and TMPDIR=/home/alex/go/tmp GOTMPDIR=/home/alex/go/tmp make build-e2e-test passed. make floci-test passed with bootstrap/state/lock and ECS runtime/candidate contract tests. No AWS account was used.
