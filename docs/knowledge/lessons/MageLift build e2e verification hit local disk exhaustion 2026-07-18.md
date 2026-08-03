---
type: lesson
title: MageLift build e2e verification hit local disk exhaustion 2026-07-18
description: 'make build-e2e-test reached the Go build step but failed with compile: writing output: no
  space left on device.'
tags:
- verification
- failure
- build
generated:
  at: '2026-07-24'
---

make build-e2e-test reached the Go build step but failed with compile: writing output: no space left on device. This is an environment capacity failure, not a repository test failure. Reclaim Docker or Go build cache space before rerunning.
