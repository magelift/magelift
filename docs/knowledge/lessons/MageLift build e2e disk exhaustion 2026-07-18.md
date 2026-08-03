---
type: lesson
title: MageLift build e2e disk exhaustion 2026-07-18
description: 'The account-free build-e2e target reached the Go CLI build but failed because the Go compiler
  could not write Pulumi AWS packages: no space left on device.'
tags:
- verification
- build
- environment
status: stable
generated:
  at: '2026-07-24'
---

The account-free build-e2e target reached the Go CLI build but failed because the Go compiler could not write Pulumi AWS packages: no space left on device. This is an environment capacity failure, separate from repository tests; reclaim Docker/Go build cache or run with a larger workspace before retrying.
