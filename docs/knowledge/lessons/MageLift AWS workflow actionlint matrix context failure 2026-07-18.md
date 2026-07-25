---
type: lesson
title: MageLift AWS workflow actionlint matrix context failure 2026-07-18
description: The first actionlint run rejected matrix.profile in top-level concurrency because the matrix
  context is unavailable there.
tags:
- github-actions
- actionlint
- failure
generated:
  at: '2026-07-24'
---

The first actionlint run rejected matrix.profile in top-level concurrency because the matrix context is unavailable there. Moving concurrency under the matrix job fixed the workflow, and actionlint now passes.
